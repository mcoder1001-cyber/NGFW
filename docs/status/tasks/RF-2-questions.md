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

## Fix round (after review 709e08e)
7. **Tunnel name length (L1).** The renderer now refuses tunnel names longer than 60 characters (so `ike-<name>` fits 64).
   Proposal for P11's contract PR: `vpn.ipsec.tunnels` keys (and `remoteAccess` profile names) `max(60)` in the schema,
   so the UI rejects them before commit instead of the renderer at tier-3 validation.
8. **D-083 "the harness re-unpacks".** The harness does not download: unpacking needs `apt-get download` (network,
   not allow-listed) — it skips with the README commands when `/run/vrx-test/<slot>/swan-stock/root` is missing
   (e.g. after a reboot). A `tools/lab` step (manager-owned) could do it; RF-2 does not add one.
9. **M2 shared-host rule for the manager:** a strongSwan renderer on a charon shared by several slots must use
   `WithOwnerPrefix(<slot prefix>)`; only the product agent uses no prefix. Please add to `docs/lab/shared-host-rules.md`.
10. **M3 for P11/DF-5 (board):** on `State.restarted` / the `daemon restarted` event, delete VPP SAs/SPD entries
    not referenced by `list-sas` (SPI + reqid), re-Apply, then `Renderer.AckRestart`.
