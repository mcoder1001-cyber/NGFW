"""Bounded source/helper controls; never host sysctls, services or PPP daemons."""
import ast
import json
from pathlib import Path
import select
import subprocess
import tempfile
import sys
import os

source = Path("apps/agent/internal/renderers/pppoe/templates/ipv6.tmpl").read_text()
with tempfile.TemporaryDirectory() as folder:
    root = Path(folder)
    substitutions = {
        "StateDir": folder,
        "HostIf": "fixture",
        "IPv6UpHook": str(root / "hook"),
        "IPv6Helper": str(root / "helper"),
        "IPv6": "slaac",
        "DhcpcdBin": str(root / "unused-client"),
        "DhcpcdConf": str(root / "unused-config"),
    }
    for key, value in substitutions.items():
        source = source.replace("{{quoted ." + key + "}}", json.dumps(value))
    source = source.replace("{{if .DefaultRoute}}True{{else}}False{{end}}", "False")
    ast.parse(source)
    print("rendered Python AST: PASS")
    namespace = {"__name__": "source_control"}
    exec(compile(source, "rendered-helper", "exec"), namespace)
    admission = namespace["ADMISSION"]
    admission.write_text("replacement\n")

    def refuse_parent(_):
        raise AssertionError("retired hook reached parent/writer operation")

    real_parent_fd = namespace["parent_fd"]
    if "--process-identity" in sys.argv:
        # This exercises the real PID/proc relationship, without starting a
        # writer or touching sysctls. Failure is evidence, never a passing skip.
        try:
            live_fd = real_parent_fd(str(os.getpid()))
        except namespace["Failure"] as exc:
            print("private live-parent identity fixture: FAIL: " + str(exc))
            raise SystemExit(1) from exc
        else:
            os.close(live_fd)
            print("private live-parent identity fixture: PASS")
            raise SystemExit(0)
    namespace["parent_fd"] = refuse_parent
    namespace["up"]("ppp0", "", "", "2", "retired")
    namespace["BLOCKED"].write_text("blocked\n")
    namespace["up"]("ppp0", "", "", "2", "replacement")
    print("retired and blocked admission: PASS")
    namespace["BLOCKED"].unlink()
    namespace["parent_fd"] = real_parent_fd

    # Model /proc returning a live reused identity while the original process
    # dies. The original pidfd must still become readable.
    namespace["identity"] = lambda _: "controlled-reused-identity"
    parent = subprocess.Popen(["sleep", "30"])
    fd = None
    try:
        fd = real_parent_fd(str(parent.pid))
        parent.kill()
        parent.wait(timeout=3)
        assert select.select([fd], [], [], 3)[0]
        assert namespace["identity"](parent.pid) == "controlled-reused-identity"
        print("immutable parent pidfd despite numeric identity reuse: PASS")
    finally:
        if parent.poll() is None:
            parent.kill()
            parent.wait(timeout=3)
        if fd is not None:
            namespace["os"].close(fd)
print("Full Go/race/golden/lifecycle execution: NOTRUN; hosted gate still required")
