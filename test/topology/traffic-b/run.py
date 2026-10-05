#!/usr/bin/env python3
"""Sequential Wave-B orchestration; deferred phases cannot produce a green wave."""
import argparse
import fcntl
import json
import os
from pathlib import Path
import secrets
import subprocess
import sys
import time

sys.dont_write_bytecode = True
from scenario import PHASES, Refused, accept, slot_values
ROOT = Path(__file__).resolve().parents[3]


def read_result(path):
    if path.is_symlink() or not path.is_file() or path.stat().st_size > 1048576:
        raise Refused("bounded regular phase result required")
    def pairs(items):
        result = {}
        for key, value in items:
            if key in result:
                raise Refused("duplicate evidence key")
            result[key] = value
        return result
    return json.loads(path.read_text(), object_pairs_hook=pairs)


def campaign(slot, output, adapters, *, execute=subprocess.run):
    values = slot_values(slot)
    run_id = secrets.token_hex(16)
    sha = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=ROOT, text=True).strip()
    output.mkdir(mode=0o700, parents=True, exist_ok=False)
    summary = {"task": "TEST-traffic-B", "slot": slot, "run_id": run_id,
               "source_sha": sha, "phases": [], "passed": False,
               "evidence_contract_only": True}
    try:
        for phase in PHASES:
            result_path = output / (phase.name + ".json")
            entry = {"phase": phase.name, "status": "deferred", "reason": phase.limitation}
            summary["phases"].append(entry)
            adapter = adapters.get(phase.name)
            if adapter is None:
                continue
            env = dict(os.environ, **values, NGFW_INTEGRATION="1", NGFW_LAB_LOCK_HELD="1",
                       NGFW_TRAFFIC_RUN_ID=run_id, NGFW_TRAFFIC_SOURCE_SHA=sha,
                       NGFW_TRAFFIC_RESULT=str(result_path), NGFW_TRAFFIC_PHASE=phase.name)
            # No shell expansion; adapters are reviewed executables, never JSON commands.
            log = output / (phase.name + ".private.log")
            fd = os.open(log, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
            try:
                with os.fdopen(fd, "wb") as stream:
                    completed = execute([str(adapter)], cwd=ROOT, env=env, stdout=stream,
                                        stderr=subprocess.STDOUT, timeout=1800, check=False)
                if completed.returncode:
                    raise Refused("driver failed; private diagnostic log retained")
                result = read_result(result_path)
                accept(phase, result, slot=slot, run_id=run_id, source_sha=sha)
                entry.update(status="evidence-contract-accepted", reason="packet assertions require independent adapter review")
            except (Refused, OSError, ValueError, subprocess.TimeoutExpired) as error:
                entry.update(status="failed", reason=str(error))
            finally:
                # Persist progress after every phase; never erase evidence after a failure.
                (output / "summary.json").write_text(json.dumps(summary, indent=2) + "\n")
        # Adapter declarations never constitute independent packet acceptance.
        return 3 if any(item["status"] == "failed" for item in summary["phases"]) else 2
    finally:
        (output / "summary.json").write_text(json.dumps(summary, indent=2) + "\n")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--slot", type=int, default=27)
    parser.add_argument("--dry-run", action="store_true")
    parser.add_argument("--output", type=Path)
    parser.add_argument("--adapter", action="append", default=[], metavar="PHASE=EXECUTABLE")
    args = parser.parse_args()
    try:
        slot_values(args.slot)
        if args.dry_run:
            print(json.dumps({"slot": args.slot, "passed": False, "mode": "plan-only", "phases": [
                {"phase": p.name, "driver": p.driver, "driver_exists": (ROOT / p.driver).is_file(),
                 "packets": p.packets, "limitation": p.limitation} for p in PHASES]}, indent=2))
            return 0
        if os.environ.get("NGFW_INTEGRATION") != "1" or os.geteuid() != 0:
            raise Refused("requires root and NGFW_INTEGRATION=1")
        adapters = {}
        for binding in args.adapter:
            name, separator, raw = binding.partition("=")
            if not separator or name not in {p.name for p in PHASES} or name in adapters:
                raise Refused("unknown or duplicate phase adapter")
            adapter = Path(raw).resolve(strict=True)
            if not adapter.is_file() or not os.access(adapter, os.X_OK):
                raise Refused("executable phase adapter required")
            adapters[name] = adapter
        output = args.output or ROOT / ".scratch" / ("traffic-b-" + time.strftime("%Y%m%d-%H%M%S") + "-" + str(os.getpid()))
        with open("/run/lock/ngfw-traffic-b-w" + str(args.slot) + ".lock", "a") as owner:
            fcntl.flock(owner, fcntl.LOCK_EX | fcntl.LOCK_NB)
            with open("/run/lock/ngfw-lab.lock", "a") as lab:
                fcntl.flock(lab, fcntl.LOCK_SH)
                return campaign(args.slot, output, adapters)
    except (Refused, OSError, ValueError) as error:
        print("REFUSED: " + str(error), file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main())
