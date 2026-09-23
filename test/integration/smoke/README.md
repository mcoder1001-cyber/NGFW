# Smoke test — ping through the host VPP over the af_packet rig

Own Go module (`ngfw/test/integration/smoke`, `replace ngfw/agent => ../../../apps/agent`) so it uses the generated
bindings without living inside the agent. Runs **only** with `VRX_INTEGRATION=1`, as root, under `flock -s
/run/lock/vrx-lab.lock`; every object carries `VRX_TEST_PREFIX`. Path recorded: **af_packet** (D-010).

```
VRX_TEST_PREFIX=w3 test/integration/smoke/run.sh          # = tools/lab lock shared go test -count=1 -v ./...
```

What it proves: govpp connects to `/run/vpp/api.sock` and `show_version` contains 26.06 · `tools/lab rig up <p>` ·
both `host-<p>l0`/`host-<p>w0` appear in `sw_interface_dump` · ping `ns-<p>-lan → ns-<p>-wan` succeeds · rx packet
counters of both host-interfaces (stats segment) increase · `rig down` leaves no netns/veth/VPP object with the prefix.
Without `VRX_INTEGRATION=1` the VPP test skips with a message (it is not a pass); `TestRigObjectMatchIsAnchored` is a pure unit
test that always runs: the leftover check must match exactly its own prefix's objects (`w1` never sees `w11`'s — review F1).
