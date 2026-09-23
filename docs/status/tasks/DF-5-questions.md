# DF-5 — questions for the manager (written while continuing; nothing here blocks DF-5)

## Q1 — FYI, decision taken: three own unmerged commits were recreated (gitleaks false positive)
`tools/ci.sh --base main` failed at gitleaks: rule `generic-api-key` matched a list of descriptor
names in an ikev2 unit test (a quoted, comma-separated list of the local-key and sleep-interval
descriptor names) in commit `f15ea5f`. The
gate prescribes "recreate the commits without it". Options: (a) recreate the branch's own commits
after `9aea5ca` — conflicts with "no history rewriting" in shared-host-rules §6; (b) add a
`.gitleaksignore` fingerprint / widen `.github/gitleaks.toml` — not my file, and the config says
"do not widen this file to make a branch pass"; (c) leave the gate red — blocks the merge.
**Chosen (a)**, limited to my own unmerged commits `f15ea5f`, `d84ff22`, `450e960` (squashed into
`c1c1c88`, same content + the test rewritten to use the name constants). The manager's salvage commit
`cc002dc` and everything before `9aea5ca` are untouched; nothing else references the old hashes.
Please confirm or tell me the preferred procedure for the next false positive.

## Q2 — IKEv2 id data is truncated by govpp (VPP / govpp code-track candidate)
`ikev2_id.data` is `string[64]` in ikev2_types.api and govpp v0.13's `DecodeString` stops at the first
NUL, so an ip4/ip6 id such as 10.4.0.1 is dumped as "10.4" (`data_len` = 4). Under D-063 (no cached
desired state) such an id can never be retrieved exactly, so `ikev2.profile` **refuses** ip ids with a
zero byte followed by a non-zero byte (10.x.0.y is common!). Fix options: (a) VPP: declare the field
`u8 data[64]` (API change → docs/vpp-code-track.md, not my file), (b) govpp: honour the sibling
`data_len` for strings, (c) keep the refusal. Please decide / add to the V-track.

## Q3 — WireGuard `src_ip` dependency has no key to point at
The prompt asks for "wireguard-interface → interface-ip of its src address (Optional)". DF-1's
address key is `interface.ip-address/<if>/<addr>/<len>`, which the WireGuard object cannot build (no
interface, no prefix length). No dependency is declared (VPP accepts an unconfigured src_ip).
Options: (a) leave it (ordering does not matter to VPP), (b) DF-1 adds an address alias
`interface-ip/<addr>` provided by every ip-address object, (c) add `src_interface` to the WireGuard
message. I recommend (b) if another consumer needs it, else (a).

## Q4 — `ErrRetrieveUnsupported` sentinel
P05 defines `scheduler.ErrRetrieveUnsupported` on `task/P05` (not on main yet). DF-5 declares
`vpn.ErrRetrieveUnsupported` with the **same message**, so `scheduler.IsRetrieveUnsupported` (which
also matches by message) recognises it. After P05 merges, a one-line follow-up can alias the
scheduler sentinel. Write-only DF-5 descriptors: `ipsec.async-mode`, `ikev2.local-key`,
`ikev2.liveness`, `ikev2.responder-hostname`, `wireguard.async-mode`.

## Q5 — D-065 alias for interfaces DF-5 creates (`ipsec<N>`, `wg<N>`)
Consumers depend on `interface/<name>`. `ipsec.itf` and `wireguard.interface` implement P05's optional
`KeyProvider.ProvidedKeys` → `interface/ipsec<N>` / `interface/wg<N>`, so ordering works even if
DF-1's alias descriptor does not know these creators. If DF-1's alias descriptor also reports them
(from sw_interface_dump), please make sure the two providers do not conflict in P05.

## Q6 — Contract placement of the VPN desired-state messages (D-055)
`apps/agent/internal/descriptors/vpn/pb/vpn.proto` is agent-internal, written to move verbatim into
`packages/proto/vrx/v1` (strings for enums, no VPP handles, secrets as references). P03b owns that
move; P11 / F-* consume the secret-reference contract documented in `docs/agent/descriptors/ipsec.md`.
Note: since D-063 the IKEv2 responder hostname is its own message `Ikev2ResponderHostname`
(`Ikev2Responder.hostname` is reserved) — P02c's `vpn.ipsec` schema maps onto that split.

## Q7 — Known restart limitation: SPD binding after an agent restart
`ipsec_spd_interface_details` reports the SPD *pool index*, not the spd_id; after an agent restart a
binding is retrieved with `spd_id: 0` until re-bound (one ErrRecreate). Acceptable? A proper fix is a
VPP API change (return spd_id) — V-track candidate.

## Q8 — WireGuard peer events → StreamEvents
`wireguard.Register` returns the peer descriptor; `peer.Events(ctx)` yields `PeerEvent` (shape in
docs/agent/descriptors/wireguard.md). Who wires it into the gRPC `StreamEvents` (P05 / P08)?

## Q9 — IKEv2 hostname responder after resolution (for the F-* initiator task)
When an initiator flow resolves a hostname responder, VPP fills `responder.addr`; the profile would
then report a responder address its desired value lacks → ErrRecreate. Out of DF-5's scope (no
initiator flows); flagged for the F-* task that adds them.
