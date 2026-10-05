#!/usr/bin/env python3
"""Actual API GRE/VXLAN -> kernel peers -> scoped tcpdump -> rollback campaign.

Run ONLY inside isolated-vpp.py; requires an already running slot API and a
private0600 bearer token file. Never accepts a shared VPP socket as its target.
"""
import argparse
import fcntl
import json
import os
from pathlib import Path
import stat
import subprocess
import sys
import urllib.request
from scenario import Refused, slot_values
ROOT = Path(__file__).resolve().parents[3]


def topology(slot, kind):
    slot_values(slot)
    prefix = f"w{slot}"
    base = slot * 1000
    underlay = f"10.{slot}.1."
    inner = f"10.{slot}.241."
    name = prefix + "-tb-" + kind
    tunnel = {"instance": base + 41, "src": underlay + "1", "dst": underlay + "2"}
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
        data = None if body is None else json.dumps(body).encode()
        req = urllib.request.Request(self.base + path, data=data, method=method,
                                     headers={"Authorization": "Bearer " + self.token, "Content-Type": "application/json"})
        with urllib.request.urlopen(req, timeout=30) as response:
            raw = response.read(1048577)
        if len(raw) > 1048576:
            raise Refused("API response exceeds bound")
        return json.loads(raw)


def check_commit(result):
    if result.get("status") != "applied" or result.get("notApplied", []) != []:
        raise Refused("configuration did not fully apply")
    if "agent.unsupported-field" in json.dumps(result):
        raise Refused("unsupported product configuration")


def run(slot, api, output):
    if os.environ.get("NGFW_DISPOSABLE_VPP") != "1":
        raise Refused("private disposable VPP required")
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
    if any(prefix in line for line in command(["ip", "netns", "list"]).splitlines()):
        raise Refused("owned namespace already exists")
    events = []
    created = False
    try:
        created = True  # rig up may fail partway; down must still run.
        command([str(ROOT / "tools/lab"), "rig", "up", prefix])
        for kind in ("gre", "vxlan"):
            patch, commands, device, destination = topology(slot, kind)
            capture = None
            applied = False
            try:
                for argv in commands:
                    peer(*argv)
                api.call("PATCH", "/config", patch)
                result = api.call("POST", "/config/commit?comment=traffic-b-" + kind)
                check_commit(result)
                applied = True
                state = api.call("GET", "/state/tunnels")
                if not any(item.get("name") == prefix + "-tb-" + kind and item.get("adminUp") for item in state.get("tunnels", [])):
                    raise Refused("tunnel state not applied")
                filter_expr = "proto 47" if kind == "gre" else "udp port 4789"
                cap = output / (kind + "-tcpdump.txt")
                with cap.open("x") as stream:
                    capture = subprocess.Popen(["ip", "netns", "exec", f"ns-{prefix}-wan", "tcpdump", "-n", "-l", "-vv", "-i", prefix + "w1", "-c", "4", filter_expr], stdout=stream, stderr=subprocess.DEVNULL)
                    try:
                        # Synchronize readiness by observing tcpdump process still alive.
                        import time
                        time.sleep(0.3)
                        if capture.poll() is not None:
                            raise Refused("tcpdump failed before probe")
                        probe = peer("ping", "-n", "-c", "4", "-W", "3", "-I", device, destination)
                        (output / (kind + "-ping.txt")).write_text(probe)
                        capture.wait(timeout=10)
                        if capture.returncode:
                            raise Refused("capture failed")
                    finally:
                        if capture.poll() is None:
                            capture.terminate()
                            try: capture.wait(timeout=5)
                            except subprocess.TimeoutExpired: capture.kill(); capture.wait()
                text = cap.read_text()
                if "ICMP echo" not in text or ("GRE" if kind == "gre" else "VXLAN") not in text:
                    raise Refused("no inner ICMP inside observed encapsulation")
                (output / (kind + "-state.json")).write_text(json.dumps(state, indent=2))
                (output / (kind + "-cli.txt")).write_text(cli("show", kind, "tunnel"))
                events.append({"phase": kind, "packets": "kernel ping and underlay encapsulated ICMP", "passed": True})
            finally:
                # Never advance to another phase after failed rollback.
                rollback = api.call("POST", f"/config/rollback/{baseline}?comment=traffic-b-rollback")
                check_commit(rollback)
                remaining = api.call("GET", "/state/tunnels")
                if any(item.get("name", "").startswith(prefix + "-tb-") for item in remaining.get("tunnels", [])):
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
        info = args.token_file.lstat()
        if not stat.S_ISREG(info.st_mode) or stat.S_IMODE(info.st_mode) != 0o600 or info.st_uid != os.geteuid():
            raise Refused("owned private0600 token file required")
        if os.environ.get("NGFW_INTEGRATION") != "1":
            raise Refused("NGFW_INTEGRATION=1 required")
        with open("/run/lock/ngfw-lab.lock", "a") as lock:
            fcntl.flock(lock, fcntl.LOCK_SH)
            run(args.slot, Api(args.slot, args.token_file.read_text().strip()), args.output)
        return 0
    except (Refused, OSError, ValueError, subprocess.SubprocessError) as error:
        print(type(error).__name__ + ": tunnel campaign failed; inspect private output", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
