# Composed Wave-A executor

`execute.py` is the in-tree composed transaction; `run.py` remains the legacy read-only
foundation entry point. No fixture becomes live proof. A manager must provision an isolated
real slot API/agent/database, a dedicated admin API key (`NGFW_HOST_ACCESS_TOKEN`), canonical
slot environment from `tools/lab env 14`, a root-owned0600 bounded manager lease at the fixed
traffic-a-lease.json path, and `/run/ngfw-test/w14/traffic-a` root-owned0700. No live execution
is authorized by a source/unit check. The manager grants the otherwise-idle traffic window.

Build finite binaries with `tools/heavy.sh` before entering that window:

```sh
tools/heavy.sh go -C test/topology/traffic-a/globals build -o traffic-a-globals .
tools/heavy.sh go -C apps/agent build -o bin/ngfw-vpp-preflight ./cmd/ngfw-vpp-preflight
NGFW_INTEGRATION=1 NGFW_TRAFFIC_A_HOST=1 python3 test/topology/traffic-a/execute.py --slot 14
python3 test/topology/traffic-a/check.py
tools/heavy.sh go -C test/topology/traffic-a/globals vet ./...
tools/heavy.sh go -C test/topology/traffic-a/globals test ./...
```

The executor refuses existing namespace/VPP slot objects, locked/dirty candidate and
preexisting enabled NAT plugins. Baseline/whole-chain commits use the real product; unsupported
or drift-ignored selected domains refuse. Tagged peer probes use20distinctTCP8000flows,
5TCP8001PBRflows,3deniedTCP8002flows, valid ping and spoof ping. Both private parent-device
pcaps are read through the existing strict checksum parser; translation, return TCP, VLAN100,
ECMP201+202, PBR202, actual denied ingress/no-output and EI stable source mapping are checked.
Capture readiness, bounded records/bytes, zero-loss tcpdump accounting and hashes are retained.

NAT plugin state is a test fixture, separate from product objects (D-071). The Go helper
remembers ownership only when both plugins started disabled, checks full running-config
fingerprint and all supported object/user/ED-VRF-table dumps before disabling its own plugin,
and refuses foreign config. ED objects are removed through one product commit before switching
the empty owned plugin to EI and committing EI objects; this adds an explicit safe transition
commit to the historical single-swap recipe. The global lock is exclusive only around fixture
changes. API/agent never gain global-owner privileges. No VPP restart/trace or host installation.

Normal cleanup restores interface baseline then original revision, stops only spawned peers,
brings our veths down and removes the owned rig. Failed applied transactions retain the down rig
and private evidence for manager recovery; no foreign candidate is discarded. PEM/token material
is never passed on argv. Evidence remains private under the allocated run directory; no pcap is
committed. The same transaction requires CLI bridge/BVI/FIB2paths/ABF/ACL/NAT bindings, strict-uRPF
drop-counter delta and counters of each allocated tagged interface. Final table/pool/interface/
namespace residue and unchanged VPP identity are mandatory. Before cleanup succeeds the
retained result is explicitly pending with whole_chain_proven=false. Only a real complete
leased execution can emit COMPOSED_PACKET_AND_READBACK_ACCEPTANCE_PASSED; source tests
never execute that path or close the host Wave-A gate.
