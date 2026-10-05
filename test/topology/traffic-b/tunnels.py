#!/usr/bin/env python3
"""Actual API GRE/VXLAN -> kernel peers -> scoped tcpdump -> rollback campaign.

Run ONLY inside isolated-vpp.py; requires an already running slot API and a
private0600 bearer token file. Never accepts a shared VPP socket as its target.
"""
import argparse
import fcntl
import hashlib
import json
import os
from pathlib import Path
import stat
import subprocess
import sys
import urllib.request
import urllib.error
from scenario import Refused, slot_values, private_identity
from probe import Capture
ROOT = Path(__file__).resolve().parents[3]


def topology(slot, kind):
    slot_values(slot)
    prefix = f"w{slot}"
    base = slot * 1000
    underlay = f"10.{slot}.2."
    inner = f"10.{slot}.241."
    name = prefix + "-tb-" + kind
    tunnel = {"enabled": True, "instance": base + 41, "src": underlay + "1", "dst": underlay + "2"}
    config = {"interfaces": {f"host-{prefix}w0": {"enabled": True, "ipv4": [underlay + "1/24"]}},
              "tunnels": {kind: {name: tunnel}}}
    if kind == "gre":
        tunnel["ipv4"] = [inner + "1/24"]
        commands = [["ip", "link", "add", "tbgre", "type", "gre", "local", underlay + "2", "remote", underlay + "1"]]
        device = "tbgre"
    elif kind == "vxlan":
        bd = prefix + "-tb-bd"
        tunnel.update(vni=base + 41, decap="l2", bridgeDomain=base + 41)
        config["routing"] = {"l2": {"bridgeDomains": {bd: {"id": base + 41}}}}
        config["interfaces"][f"loop{slot}41"] = {"enabled": True, "ipv4": [inner + "1/24"], "l2": {"bridgeDomain": bd, "bvi": True}}
        commands = [["ip", "link", "add", "tbvx", "type", "vxlan", "id", str(base + 41),
                     "local", underlay + "2", "remote", underlay + "1", "dev", prefix + "w1", "dstport", "4789"]]
        device = "tbvx"
    else:
        raise Refused("GRE or VXLAN required")
    commands += [["ip", "addr", "add", inner + "2/24", "dev", device], ["ip", "link", "set", device, "up"]]
    return config, commands, device, inner + "1"


class Api:
    def __init__(self, slot, token):
        self.base = "http://127.0.0.1:" + slot_values(slot)["NGFW_HTTP_PORT"] + "/api/v1"
        self.token = token
    def call(self, method, path, body=None):
        if method == "POST" and body is None:
            body = {}
        data = None if body is None else json.dumps(body).encode()
        req = urllib.request.Request(self.base + path, data=data, method=method,
                                     headers={"Authorization": "Bearer " + self.token, "Content-Type": "application/merge-patch+json" if method == "PATCH" else "application/json"})
        try:
            response = urllib.request.urlopen(req, timeout=30)
        except urllib.error.HTTPError as error:
            raw = error.read(1048576)
            try:problem=json.loads(raw)
            except ValueError:problem={}
            raise Refused(f"API HTTP {error.code} {method} {path}; problem {problem.get('type','unknown')} pointers {[e.get('pointer') for e in problem.get('errors',[])]} title {problem.get('title')} detail {problem.get('detail')}") from None
        with response:
            raw = response.read(1048577)
        if len(raw) > 1048576:
            raise Refused("API response exceeds bound")
        return json.loads(raw)


def check_commit(result, *, baseline_warnings=None, changed_paths=()):
    if result.get("status") != "applied" or result.get("notApplied", []) != []:
        raise Refused("configuration did not fully apply")
    warnings = result.get("warnings", [])
    unsupported = [w for w in warnings if w.get("rule") == "agent.unsupported-field"]
    if "agent.unsupported-field" in json.dumps(result) and not unsupported:
        raise Refused("unsupported result outside structured warnings")
    for warning in unsupported:
        pointer = warning.get("pointer", "")
        changed = any(pointer == p or pointer.startswith(p + "/") or p.startswith(pointer + "/") for p in changed_paths)
        if changed or (baseline_warnings is not None and warning not in baseline_warnings) or (baseline_warnings is None and not changed_paths):
            raise Refused("unsupported changed or unrecognized configuration: " + str(warning))
    revision = result.get("revision", {}).get("id")
    if type(revision) is not int or revision <= 0:
        raise Refused("concrete applied revision required")
    return warnings


def run(slot, api, output, baseline_warnings):
    if os.environ.get("NGFW_DISPOSABLE_VPP") != "1":
        raise Refused("private disposable VPP required")
    identity = private_identity()
    prefix = f"w{slot}"
    output.mkdir(mode=0o700, parents=True, exist_ok=False)
    def command(argv):
        return subprocess.check_output(argv, stderr=subprocess.STDOUT, timeout=40, text=True)
    def cli(*args):
        return command(["timeout", "10", "vppctl", *args])
    def peer(*args):
        return command(["ip", "netns", "exec", f"ns-{prefix}-wan", *args])
    original = api.call("GET", "/config/diff")
    if original.get("changes"):
        raise Refused("slot candidate already dirty")
    baseline = original["baseRevision"]
    if type(baseline) is not int or baseline <= 0:
        raise Refused("concrete original slot revision required")
    if any(prefix in line for line in command(["ip", "netns", "list"]).splitlines()):
        raise Refused("owned namespace already exists")
    events = []
    created = False
    try:
        created = True  # rig up may fail partway; down must still run.
        command([str(ROOT / "tools/lab"), "rig", "up", prefix])
        # Rig creates kernel peers and VPP ports; the REST candidate must own
        # creation of its WAN port rather than collide with an unowned object.
        cli("delete", "host-interface", "name", prefix + "w0")
        for kind in ("gre", "vxlan"):
            patch, commands, device, destination = topology(slot, kind)
            capture = None
            try:
                for argv in commands:
                    peer(*argv)
                lock=api.call("GET","/config/lock")
                current=api.call("GET","/config/diff")
                if lock.get("locked") or current.get("changes"):
                    raise Refused("foreign candidate or candidate ownership")
                api.call("PATCH","/config",{})
                claimed=api.call("GET","/config/lock")
                if not claimed.get("locked") or not claimed.get("ownerId"):
                    raise Refused("interactive candidate lock not acquired")
                if api.call("GET","/config/diff")!=current:
                    raise Refused("running revision changed during claim")
                api.call("PATCH", "/config", patch)
                candidate=api.call("GET","/config/candidate")
                digest=hashlib.sha256(json.dumps(candidate,sort_keys=True,separators=(",",":")).encode()).hexdigest()
                observed_lock=api.call("GET","/config/lock")
                if not observed_lock.get("locked") or observed_lock.get("ownerId") != claimed.get("ownerId"):
                    raise Refused("candidate ownership changed before commit")
                result = api.call("POST", "/config/commit?comment=traffic-b-" + kind)
                check_commit(result, baseline_warnings=baseline_warnings, changed_paths=("/tunnels", "/interfaces", "/routing/l2"))
                running=api.call("GET","/config")
                if hashlib.sha256(json.dumps(running,sort_keys=True,separators=(",",":")).encode()).hexdigest()!=digest:
                    raise Refused("committed document differs from observed candidate")
                state = api.call("GET", "/state/tunnels")
                if not any(item.get("name") == prefix + "-tb-" + kind and item.get("adminUp") for item in state.get("items", [])):
                    raise Refused("tunnel state not applied: " + json.dumps(state))
                filter_expr = "proto 47" if kind == "gre" else "udp port 4789"
                cap = output / (kind + "-tcpdump.txt")
                capture = Capture(f"ns-{prefix}-wan", prefix + "w1", filter_expr, cap)
                try:
                    probe = peer("ping", "-n", "-c", "4", "-W", "3", "-I", device, destination)
                    (output / (kind + "-ping.txt")).write_text(probe)
                finally:
                    capture.close(); capture = None
                text = cap.read_text()
                if "ICMP echo" not in text or ("GRE" if kind == "gre" else "VXLAN") not in text:
                    raise Refused("no inner ICMP inside observed encapsulation")
                (output / (kind + "-state.json")).write_text(json.dumps(state, indent=2))
                (output / (kind + "-cli.txt")).write_text(cli("show", kind, "tunnel"))
                events.append({"phase": kind, "packets": "kernel ping and underlay encapsulated ICMP", "passed": True, "revision":result["revision"]["id"],"candidate_sha256":digest,"unrelated_baseline_warnings":baseline_warnings,"private_vpp":identity})
            finally:
                # Never advance to another phase after failed rollback.
                api.call("POST", "/config/discard")
                rollback = api.call("POST", f"/config/rollback/{baseline}?comment=traffic-b-rollback")
                check_commit(rollback, baseline_warnings=baseline_warnings, changed_paths=("/tunnels", "/interfaces", "/routing/l2"))
                remaining = api.call("GET", "/state/tunnels")
                if any(item.get("name", "").startswith(prefix + "-tb-") for item in remaining.get("items", [])):
                    raise Refused("owned tunnel residue after rollback")
                peer("ip", "link", "del", device)
    finally:
        if created:
            command([str(ROOT / "tools/lab"), "rig", "down", prefix])
        if any(prefix in line for line in command(["ip", "netns", "list"]).splitlines()):
            raise Refused("namespace residue")
        (output / "summary.json").write_text(json.dumps({"phases": events, "whole_wave_passed": False}, indent=2))
    return events


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--slot", type=int, required=True)
    parser.add_argument("--token-file", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    try:
        fd = os.open(args.token_file, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
        try:
            info = os.fstat(fd)
            if not stat.S_ISREG(info.st_mode) or stat.S_IMODE(info.st_mode) != 0o600 or info.st_uid != os.geteuid() or info.st_size > 8192:
                raise Refused("owned private0600 bounded token file required")
            token = os.read(fd, 8193).decode().strip()
        finally:
            os.close(fd)
        if os.environ.get("NGFW_INTEGRATION") != "1":
            raise Refused("NGFW_INTEGRATION=1 required")
        with open("/run/lock/ngfw-lab.lock", "a") as lock:
            fcntl.flock(lock, fcntl.LOCK_SH)
            api=Api(args.slot,token)
            # Standalone CLI observes the applied baseline instead of inventing warnings.
            current=api.call('GET','/config/diff')
            if current.get('changes') or type(current.get('baseRevision')) is not int:raise Refused('clean concrete baseline required')
            dry=api.call('POST','/config/validate')
            baseline_warnings=dry.get('warnings',[])
            run(args.slot,api,args.output,baseline_warnings)
        return 0
    except (Refused, OSError, ValueError, subprocess.SubprocessError) as error:
        print(type(error).__name__ + ": tunnel campaign failed; inspect private output", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
