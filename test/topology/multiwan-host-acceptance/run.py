#!/usr/bin/env python3
"""Real Multi-WAN acceptance: private VPP/agent network namespace, real external API."""
import os, json, shlex, shutil, subprocess, tempfile, hashlib, fcntl, stat
from pathlib import Path
ROOT=Path(__file__).resolve().parents[3]
PRODUCT=Path(os.environ.get("NGFW_MULTIWAN_PRODUCT_ROOT",str(ROOT)))
BIN=Path(os.environ["NGFW_MULTIWAN_BIN_DIR"])
TEST_BIN=os.environ.get("NGFW_MULTIWAN_TEST_BIN")
if TEST_BIN:
 test_binary=Path(TEST_BIN)
 info=test_binary.lstat()
 if not test_binary.is_absolute() or not stat.S_ISREG(info.st_mode) or info.st_uid!=0 or info.st_nlink!=1 or info.st_mode&0o022 or not os.access(test_binary,os.X_OK):raise SystemExit("unsafe precompiled test binary")
 parent=test_binary.parent
 while str(parent) not in ("/", "/tmp", "/dev/shm"):
  directory=parent.lstat()
  if not stat.S_ISDIR(directory.st_mode) or directory.st_uid!=0 or directory.st_mode&0o022:raise SystemExit("unsafe test binary ancestor")
  parent=parent.parent
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
# A random per-stack key prefix isolates keys when the host has only 16 DBs.
if "NGFW_MULTIWAN_VALKEY_DB" in env:
 db=int(env["NGFW_MULTIWAN_VALKEY_DB"])
 if db not in range(16):raise SystemExit("invalid laboratory Valkey DB")
 env["NGFW_VALKEY_DB"]=str(db)
subprocess.run(["ip","netns","add",NS],check=True)
try:
 with tempfile.TemporaryDirectory(prefix="multiwan-host-") as tmp:
  stage=Path(tmp)
  for path in (ROOT/"test/topology/acl").glob("*"):
   if path.suffix==".go" or path.name in ("go.mod","go.sum"):shutil.copy2(path,stage/path.name)
  mod=stage/"go.mod";mod.write_text(mod.read_text().replace("../../../apps/agent",str(ROOT/"apps/agent")))
  stack=stage/"stack_test.go";stack.write_text(stack.read_text().replace("dir, err := os.Getwd()",'dir, err := os.Getwd()\n\tdir = '+json.dumps(str(PRODUCT))))
  if env.get("NGFW_MULTIWAN_EXTENDED")=="1" and env.get("NGFW_MULTIWAN_PRIVATE_NAT_PREREQUISITE")!="1":
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
  acceptance_source=acceptance.read_text().replace("w7",f"w{SLOT}").replace("10.7.",f"10.{SLOT}.")
  if env.get("NGFW_MULTIWAN_PRIVATE_NAT_PREREQUISITE")=="1":
   anchor="st := newStack(t, s)"
   if acceptance_source.count(anchor)!=1:raise SystemExit("private NAT setup anchor changed")
   acceptance_source=acceptance_source.replace(anchor,'t.Log("private NAT engine prerequisite; engine enable installation excluded from acceptance")\n\tmustRun(t, "flock", "-x", "/run/lock/ngfw-globals.lock", "vppctl", "nat44", "plugin", "enable", "sessions", "4096")\n\t'+anchor)
  (stage/acceptance.name).write_text(acceptance_source)
  extension=Path(__file__).with_name("extended_test.go")
  if extension.exists():(stage/extension.name).write_text(extension.read_text().replace("w7",f"w{SLOT}").replace("10.7.",f"10.{SLOT}.").replace("IP4Address{10, 7,",f"IP4Address{{10, {SLOT},"))
  command=[TEST_BIN,"-test.v","-test.count=1","-test.timeout=4m","-test.run=^TestMultiWANRealAPI$"] if TEST_BIN else ["go","-C",tmp,"test","-v","-count=1","-timeout","4m","-run","TestMultiWANRealAPI","."]
  completed=subprocess.run([ROOT/"tools/heavy.sh","python3",ROOT/"test/topology/hardware-smoke/isolated-vpp.py",*command],env=env,capture_output=True,text=True)
  print(completed.stdout,end="",flush=True);print(completed.stderr,end="",flush=True)
  lines=completed.stdout.splitlines()
  passed=any(line=="=== RUN   TestMultiWANRealAPI" for line in lines) and any(line.startswith("--- PASS: TestMultiWANRealAPI (") for line in lines) and not any("--- SKIP:" in line for line in lines)
  result=completed.returncode or (0 if passed else 1)
finally:
 subprocess.run(["ip","netns","del",NS],check=True)
raise SystemExit(result)
