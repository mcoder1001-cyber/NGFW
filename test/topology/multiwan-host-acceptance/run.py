#!/usr/bin/env python3
"""Real Multi-WAN acceptance: private VPP/agent network namespace, real external API."""
import os, json, shlex, shutil, subprocess, tempfile, hashlib, fcntl
from pathlib import Path
ROOT=Path(__file__).resolve().parents[3]
PRODUCT=Path(os.environ.get("NGFW_MULTIWAN_PRODUCT_ROOT",str(ROOT)))
BIN=Path(os.environ["NGFW_MULTIWAN_BIN_DIR"])
SLOT=int(os.environ.get("NGFW_MULTIWAN_SLOT", "7"))
if SLOT not in (*range(1,12), *range(14,33)):
 raise SystemExit("invalid developer slot")
NS=f"ns-w{SLOT}-mw-router"
slot_lock=open(f"/run/lock/ngfw-slot-{SLOT}.lock", "a")
fcntl.flock(slot_lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
for name in ("ngfw-agent","ngfw-agentctl","ngfw-vpp-preflight"):
 binary=BIN/name
 if not binary.is_file() or not os.access(binary,os.X_OK):raise SystemExit("missing executable: "+str(binary))
 print(name+" sha256="+hashlib.sha256(binary.read_bytes()).hexdigest(),flush=True)
env=dict(os.environ)
for line in subprocess.check_output([ROOT/"tools/lab","env",str(SLOT)],text=True).splitlines():
 if line.startswith("export "):
  key,value=line[7:].split("=",1);env[key]=shlex.split(value)[0]
env["NGFW_INTEGRATION"]="1"
subprocess.run(["ip","netns","add",NS],check=True)
try:
 with tempfile.TemporaryDirectory(prefix="multiwan-host-") as tmp:
  stage=Path(tmp)
  for path in (ROOT/"test/topology/acl").glob("*"):
   if path.suffix==".go" or path.name in ("go.mod","go.sum"):shutil.copy2(path,stage/path.name)
  mod=stage/"go.mod";mod.write_text(mod.read_text().replace("../../../apps/agent",str(ROOT/"apps/agent")))
  stack=stage/"stack_test.go";stack.write_text(stack.read_text().replace("dir, err := os.Getwd()",'dir, err := os.Getwd()\n\tdir = '+json.dumps(str(PRODUCT))))
  if env.get("NGFW_MULTIWAN_EXTENDED")=="1":
   acl=stage/"acl_test.go"
   acl.write_text(acl.read_text().replace('"NGFW_GLOBALS_OWNER=0"','"NGFW_GLOBALS_OWNER=1"'))
  for name in ("vpp","ngfw-agent","ngfw-vpp-preflight"):
   executable=shutil.which("vpp") if name=="vpp" else str(BIN/name)
   wrapper=stage/name;wrapper.write_text("#!/bin/sh\nexec ip netns exec "+NS+" "+shlex.quote(executable)+' "$@"\n');wrapper.chmod(0o755)
  env["PATH"]=str(stage)+":"+env["PATH"]
  env["NGFW_ACL_AGENT_BIN"]=str(stage/"ngfw-agent")
  env["NGFW_ACL_PREFLIGHT_BIN"]=str(stage/"ngfw-vpp-preflight")
  env["NGFW_ACL_AGENTCTL_BIN"]=str(BIN/"ngfw-agentctl")
  acceptance=Path(__file__).with_name("acceptance_test.go")
  (stage/acceptance.name).write_text(acceptance.read_text().replace("w7",f"w{SLOT}").replace("10.7.",f"10.{SLOT}."))
  extension=Path(__file__).with_name("extended_test.go")
  if extension.exists():(stage/extension.name).write_text(extension.read_text().replace("w7",f"w{SLOT}").replace("10.7.",f"10.{SLOT}."))
  result=subprocess.call([ROOT/"tools/heavy.sh","python3",ROOT/"test/topology/hardware-smoke/isolated-vpp.py","go","-C",tmp,"test","-v","-count=1","-timeout","4m","-run","TestMultiWANRealAPI","."],env=env)
finally:
 subprocess.run(["ip","netns","del",NS],check=True)
raise SystemExit(result)
