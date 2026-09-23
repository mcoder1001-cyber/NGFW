# RF-2 — questions (none blocking; work continued)

1. **strongSwan not installed on the host.** The prompt says the stock package is installed (disabled); it is not. I used
   extracted debs under `/run/vrx-test/w3/swan-stock` (no install). Should the manager install the stock package (disabled,
   masked) for RF-* / P11 test runs, or keep the extract-into-/run approach (tmpfs, redo after reboot; README commands)?
   CI slot 12 (`tools/ci.sh full`) will **skip** the integration test until one of the two exists for `w12`.
2. **go.mod edited** outside my file set: `github.com/strongswan/govici v0.8.2` (MIT, required by the task) and
   `golang.org/x/sys` moved indirect → direct (harness setns). OK?
3. **ALLOWLIST.md** edited (expected per envelope): swanctl Planned→Active, test-only rows for `ip` (strongswan harness) and
   `charon-systemd`.
4. **No IPsec EventKind / state message** in the proto: events are `EVENT_KIND_UNSPECIFIED` + attributes (same gap as RF-1 Q3),
   Retrieve returns `structpb` (D-055). P11's contract PR should add `IpsecSaState` and SA up/down kinds.
5. **Object names with `.`**: mapped to `+` in strongSwan (decision in RF-2.md). Alternative: forbid `.` in
   `vpn.ipsec.tunnels` keys in the schema (P02c/P11 contract).
6. **Kernel-vpp crash residue**: after a charon crash its SAs stay in the data plane (seen with kernel-netlink xfrm). P11/DF-5
   should reconcile VPP SAs against `Retrieve` on charon restart.
