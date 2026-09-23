# Descriptors — how to write one (DF-*, P05 core)

A descriptor is the reconciler's adapter for one VPP object type: `KeyOf`, `Dependencies`,
`Create`, `Update`, `Delete`, `Retrieve` (docs/01-architecture.md AD-3). The scheduler
(task P05) diffs desired against `Retrieve()` and calls these in dependency order; you never
decide *whether* to create, only *how*. The contract is `internal/scheduler/descriptor.go`
(frozen; changes = `contract/<id>` branch) and the worked example is
`internal/scheduler/example_descriptor_test.go`. This file answers *where do I put files, how
do I name objects, how do I test without touching other workers' objects*.

## Where files go

```
apps/agent/internal/descriptors/<plugin>/          one Go package per VPP plugin / api file
  <object>.go                                      type <Object>Descriptor struct{ client vpp.Client; owner string }
  <object>_test.go                                 unit tests with internal/vpp/fake
  <object>_integration_test.go                     VRX_INTEGRATION=1 tests against the host VPP
  register.go                                      func Register(r scheduler.Registry, c vpp.Client, owner string)
docs/agent/descriptors/<plugin>.md                 table: object type ↔ VPP messages ↔ notes/limitations
```

`Register` is the only entry point P05 wires: it constructs every descriptor of the plugin
with the shared client and owner and calls `r.Register(d)` (panics on a duplicate name, which
is a programming error). Core descriptors owned by P05 live in `descriptors/core/`; everything
interface-family is DF-1/DF-2; see your task prompt for the exact list and do not build
descriptors another task owns.

## Naming

- **Descriptor name** (`Name()`): `<plugin>.<object>` in lower-case, digits, `-` and `.`:
  `interface.loopback`, `interface.ip-address`, `ip.route`, `ip.table`, `nat44-ed.static-mapping`,
  `acl.acl`, `ipsec.sa`. Unique across the agent (`scheduler.ValidName` is the rule).
- **Key** (`KeyOf(obj)`): `scheduler.Join(Name(), <stable id parts>...)`, e.g.
  `interface.loopback/loop200`, `interface.ip-address/loop200/10.2.1.1/24`,
  `ip.route/2000/10.2.0.0/24/10.2.1.254`. The id is what a human would call the object and is
  stable across VPP restarts; runtime handles (sw_if_index, table index) go in `Meta`, never in
  the key. Canonicalise addresses with `net/netip` so `10.2.1.1/24` and `010.2.1.1/24` produce
  one key.
- **VPP message names** come only from the generated bindings in `apps/agent/binapi/<plugin>`
  (P04, manager-owned). Use the generated typed client — `svc := interfaces.NewServiceClient(client)`
  — so message names, field names and `Retval` handling are compile-checked. Never `vppctl`,
  never `exec.Command`, never a `vl_api_*` name typed by hand. If a message you need is missing
  from binapi, write `docs/status/tasks/<id>-questions.md` and continue; do not regenerate.

## Ownership: one VPP, many agents

The host VPP is shared by up to 12 workers' tests and, in production, by one agent. Every
object you create is stamped with the **owner** the descriptor was constructed with
(`VRX_OWNER`, default `vrx`; tests pass their `VRX_TEST_PREFIX`, e.g. `w2`):

- Interfaces: `sw_interface_tag_add_del` with `vpp.OwnerTag(owner, key.ID())` →
  `"w2:loop200"` (≤ 63 bytes). `Retrieve` keeps only dumps whose tag parses with
  `vpp.ParseOwnerTag(tag, owner)`.
- Objects without a tag (tables, routes, NAT pools, ACLs): use the owner-prefixed name/tag
  field where the API has one (`ip_table_add_del.name`, `acl_add_replace.tag`), otherwise the
  slot's numeric range (tables `VRX_VPP_TABLE_BASE..+999`, NAT pools `10.<slot>.0.0/16`) and
  an owner table P05 keeps in the state dir. State the mechanism in your plugin doc.

`Retrieve` must return **all** owned objects, including ones not in the desired state — that is
how the scheduler deletes leftovers and drift. It must return **no** object of another owner —
that is how two agents coexist. `local0` and anything unprefixed are never yours.

## Retrieve rules

1. Full dump (`*_dump` → `*_details` until `control_ping_reply`; the generated
   `XxxDump(ctx, req)` client returns a stream whose `Recv()` ends with `io.EOF`).
2. Filter by owner (above).
3. Decode into the **same proto type** as the desired state, with the **same canonical
   form**: addresses via `netip`, repeated fields sorted, MACs lower-case, defaults filled the
   way the API layer fills them. The scheduler diffs with `proto.Equal`; any field you decode
   differently from how it is desired causes a perpetual Update. Do not include read-only status
   (link state, counters) in the Value.
4. Fill `Meta` exactly as `Create` would (`sw_if_index`, …). Meta is not persisted; after a
   restart the scheduler only has what Retrieve returns.
5. If the plugin is not loaded on this host (`docs/lab/host-vrx-a.md`: `linux_cp`, `linux_nl`,
   `npt66`), return a typed error the integration test recognises and `t.Skip`s on.

`Update` changes what VPP can change in place and returns `scheduler.ErrRecreate` for anything
else (interface type, table id, tunnel endpoints). Don't emulate in-place updates with
delete+create inside the descriptor — the scheduler does that and re-creates dependents.

## Tests

**Unit** (`make test`, no VPP): table-driven with `internal/vpp/fake`:

```go
f := fake.New(fake.WithControlPingReply(&memclnt.ControlPingReply{}))
f.Reply("create_loopback_instance", &interfaces.CreateLoopbackInstanceReply{SwIfIndex: 5})
f.On("sw_interface_dump", func(api.Message) ([]api.Message, error) { return []api.Message{...}, nil })
d := loopback.New(f, "w2")
```

Cover: create (the exact request via `f.CallsNamed`), idempotent re-apply (Retrieve equals
desired → the scheduler plans nothing), update in place, `ErrRecreate`, delete with the Meta
from Create, Retrieve decoding incl. another owner's objects being filtered, dependency
ordering (`Dependencies` output), VPP errors (`Retval != 0`, `vpp.ErrDisconnected`). Stateful
handlers (closures that keep a map of "VPP" objects) make a dump reflect earlier creates — see
the example test.

**Integration** (`VRX_INTEGRATION=1`, on the host, under the shared lab lock) —
`docs/lab/shared-host-rules.md` is mandatory:

```go
func TestLoopbackOnHost(t *testing.T) {
    vpptest.SkipUnlessIntegration(t)
    vpptest.LockLab(t)                               // flock -s /run/lock/vrx-lab.lock
    owner := vpptest.Prefix(t)                       // "w2"
    inst := vpptest.LoopbackInstance(t, 1)           // 201 → loop201
    vrf := vpptest.TableBase(t) + 1                  // 2001
    // connect with govpp to /run/vpp/api.sock (P05 provides the client; until then socketclient + core.Connect)
    t.Cleanup(func() { /* Delete exactly what you created, by the Meta you got back */ })
    // create → Retrieve shows it (filter by owner!) → delete → Retrieve shows nothing of ours
}
```

- Every name, tag, table id and address is inside your slot: prefix `w<N>`, loopbacks
  `loop<N>00..<N>99`, tables `N000..N999`, NAT pools `10.N.0.0/16`, host-interfaces/veths
  `w<N>-*`. `vpptest` computes them from the envelope's environment and refuses to run
  unprefixed in integration mode.
- Assertions on Retrieve filter by owner — other workers' objects are on the same VPP at the
  same time; counting all loopbacks is wrong.
- Clean up in `t.Cleanup` by the handles you hold; the manager's nightly sweep deletes leftovers
  by prefix and files an issue against the slot.
- Never `local0`, never anything unprefixed, never `vppctl clear ...` globally, never a VPP
  restart (D-012). Restart-safety is proven by stopping *your* agent process, deleting *your*
  objects via binapi and starting it again.
- Plugin not loaded → `t.Skip` with the reason; it must not fail the gate.
