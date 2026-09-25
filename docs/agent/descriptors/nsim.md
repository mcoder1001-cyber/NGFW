# Descriptors — nsim (F-loopback-bvi-gso-lldp-span, WBS D1.9 — a lab tool)

Package `apps/agent/internal/descriptors/nsim`, built on `descriptors/dfkit` (D-077): values are dfkit structpb specs
(`Config`, `CrossConnect`, `Output`). `nsim.RegisterGlobals(r, client, owner, store)` registers all three — **only the
globals owner calls it** (D-071): VPP holds one delay model and one cross-connect pair (`nsim_main_t`), and an output
feature without this owner's model would run on another agent's parameters. `store` is the owner's persisted D-076
BootStore (`CheckPersistent`, TD-11b). Messages only from `apps/agent/binapi/nsim`.

| Object type (descriptor) | Key | Depends on | VPP messages | Update | Notes / limitations |
|---|---|---|---|---|---|
| `nsim.config` | `nsim.config/global` | — | `nsim_configure2` (delay µs, average packet size, bandwidth bit/s, packets per drop; reorder 0) | Create again (VPP frees and reallocates the wheels: frames in flight are lost) | **Write-only** (no getter, D-063). Applied once per VPP boot and value (record `<delay>/<bps>/<size>/<ppd>`), so resyncs never reallocate. VPP refuses delay 0, bandwidth 0 and a packet size outside 64–9000. **VPP cannot unconfigure nsim**: Delete only forgets the record; the model stays, inert without a cross-connect or an output interface. |
| `nsim.cross-connect` | `nsim.cross-connect/global` | `nsim.config/global` (VPP answers -76 before the model), `interface/<a>`, `interface/<b>` | `nsim_cross_connect_enable_disable` | ErrRecreate | Write-only. Hardware interfaces only (VPP rejects sub-interfaces). The device-input feature stacks on every enable → applied once per boot and pair (record `<idx>/<a>,<idx>/<b>`); Delete disables only with this boot's record. Claims on untagged interfaces. |
| `nsim.output` | `nsim.output/<interface>` | `nsim.config/global`, `interface/<name>` | `nsim_output_feature_enable_disable` | none (the value is the interface) | Write-only. Hardware interfaces only; the interface-output feature stacks → applied once per boot, index and name; Delete disables only with this boot's record. |

Projection (`internal/desired/nsim.go`): `services.nsim` → `nsim.config/global` (ms → µs, Mbit/s → bit/s, drop fraction →
packets per drop, packet size default 1500), `crossConnect` → `nsim.cross-connect/global`, `outputInterfaces[i]` →
`nsim.output/<if>`. A slot agent (not the globals owner) projects nothing and reports `/services/nsim` as
`agent.unsupported-field`; the globals owner notes it `agent.write-only` for the drift view.

Unit tests (`nsim_test.go`, a fake modelling -76 before the model and the stacking features): `TestConfigAppliedOncePerValue`,
`TestCrossConnectAndOutput`; agent level `TestLoopbackBviGsoLldpSpanGlobalsOwner`. The host test `TestNsimOnHost` is
**opt-in** (`VRX_NSIM_HOST=1`, globals lock exclusive, D-082): the previous model cannot be read back to restore it
(shared-host-rules §7), so it runs only in a manager VPP window. The default gate's nsim evidence is the fake client.
