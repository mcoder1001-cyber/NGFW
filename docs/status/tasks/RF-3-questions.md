# RF-3 — open questions (worker did not wait; each has the choice taken)

**Q1 — host `chrony.service` is active + enabled.** The envelope/acceptance expects
`systemctl is-active … chrony` = inactive. On this host chrony is the host's own time sync
(active since boot, enabled). RF-3 never touched it (no stop/disable: stopping it would change
host time behaviour and is a system-unit action the envelope forbids). kea-dhcp4-server,
kea-dhcp6-server, kea-ctrl-agent, unbound are inactive + disabled and stayed so.
→ Manager: accept "chrony.service untouched (pre-existing host timesync)" or decide who may stop it.

**Q2 — no state messages for DHCP / DNS / NTP in the proto.** `Retrieve` returns a
`structpb.Struct` (same stand-in as RF-1, D-055): Kea `{dhcp4, dhcp6}` with status/config/
statistics/leases, Unbound `{running, status, stats, forwards, stubs, localZones, localData}`,
chrony `{running, tracking, sources, sourceStats, serverStats}`. Typed Go structs exist
(`unbound.State`, `chrony.State`, `kea.State`). → P03b / F-dhcp / F-dns / F-ntp: add
`DhcpState`/`DnsState`/`NtpState` messages (additive contract) when the state API lands.

**Q3 — `ActionRequired` (start/restart) is a per-package typed error, not in `renderer.go`.**
The contract has no "needs restart" result. Each package returns `*ActionRequired` with method
`NeedsRestart() (unit, action string)`; the commit engine can match the interface
`interface{ NeedsRestart() (string, string) }` with `errors.As`. Files stay written (not a
failure). Options: (a) keep duck-typed per package [taken]; (b) contract change: a shared
`renderers.ActionRequired` in renderer.go (RF-1/P05 owners). → P05 (commit engine) to confirm.

**Q4 — schema gaps found while rendering (additive `contract/` candidates, not done here):**
- `services.ntp.servers[]` has no `port` (the test uses `Paths.SourcePort`); no `peer` entries.
- `services.dns` has no stub zones (`stub-zone:` is not rendered; `list_stubs` is still retrieved).
- `services.dhcp` has no `option-def` / option type, no relay-agent (`relay`) per subnet, no
  client-id reservations (a v4 `duid` is rejected, matching the schema).
- `DhcpOption.data` allows 1024 chars; the RF-3 prompt says ≤ 255 — the renderer takes 255 for
  DHCPv4 (wire limit of one option) and 1024 for DHCPv6.
→ F-dhcp / F-dns / F-ntp owners.

**Q5 — `services.ntp.ntsServer` is refused by the renderer** (`ErrInvalid`: NTS-KE server
certificates are F-ntp). A document that sets it cannot be committed until F-ntp extends the
renderer. Alternative: ignore with a warning. → F-ntp.

**Q6 — per-VRF daemon instances.** Kea (per family) and Unbound run one instance; all
enabled servers/resolvers must share one VRF, else `ErrInvalid`. Multiple Unbound resolvers are
separated by `view:` + `interface-view`. Per-VRF processes (netns per VRF) need a namespace
launcher (the test-only `ip netns exec` is deliberately not in production allowlists). → VDOM /
multi-VRF follow-up.

**Q7 — ALLOWLIST.md is outside the envelope's file set** but the task prompt requires the rows
and `TestAllowlistDocumented` fails without them. RF-3 added one new section
("Active — RF-3") and removed its own rows from "Planned"; RF-1 edits the same file (other rows) —
expect a trivial merge.
