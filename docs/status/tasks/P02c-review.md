# P02c — review (schema group (c): vpn, tunnels, services, ha)

Reviewer: review agent, 2026-09-23. Branch `task/P02c` (tip `2e0d449` before the review merge), worktree `/root/ngfw-wt/P02c`.
Read: 00-CONTEXT, REVIEW-PROMPT, P02 prompt (group (c)), docs/04, vdom.md, LOG D-017…D-021/D-024/D-036/D-037, wbs D6/D7/D9, P11 §2;
`docs/status/tasks/P02c*.md`, `docs/contracts/schema-vpn-tunnels-services-ha.md`, all four domain files, all five validator files, all nine
test files, all 16 fixtures, plus main's `index.ts`/`index.test.ts`/`ui.ts`/`registry.ts`/`pointer.ts`/`examples.test.ts` and the sibling
branches `task/P02a`, `task/P02b` (export lists, `git merge-tree`).

## What I ran (host `ngfw`)

- Ownership: `git diff --name-only main...task/P02c` = 40 files, all inside the allowed set (4 domain files + tests, 5 semantic files + tests
  + `vpn.fixtures.ts`, 16 fixtures, the group-(c) contract doc, `docs/status/tasks/P02c*`). No `index.ts`/`primitives.ts`/`gen.ts`/other
  domain edits. Contract commit present: `a306c2b contract(schema): model vpn, tunnels, services and ha domains …`. No scope creep.
- `git merge -q main` into the review worktree → clean (`ffcff8a`). `pnpm --filter @ngfw/schema test` on the merged tree: **20 files,
  483 tests passed**, including main's D-036 `index.test.ts` (5 tests) — item (8) OK (`/root/ngfw-wt/P02c-review-gate.log`).
- `tools/ci.sh --base main` on the merged tree: **CI GATE FAILED** at "forbidden patterns (+ gitleaks)", `EXIT=1`
  (`/root/ngfw-wt/P02c-review-gate2.log`, run dir `/root/ngfw-wt/logs/ci/P02c-20260923-154044-445252`) — see F1.
- Validators against the four `*-semantic-*` fixtures (vitest scratch file, deleted afterwards) — pointers correct, exactly one finding each:
  ```
  vpn-semantic-missing-proposal.json          /vpn/ipsec/tunnels/site-b/proposal
  tunnels-semantic-source-not-configured.json /tunnels/gre/gre-x/src
  services-semantic-dhcp-subnet-outside-interface.json /services/dhcp/servers/lan/subnets/wrong/subnet
  ha-semantic-duplicate-vrid.json             /ha/vrrp/1/vrId
  ```
  and all 7 valid fixtures → 0 findings; 26 group-(c) validators registered. Adversarial probes a–o below.

## Findings (ranked)

### F1 — BLOCKING: the gate fails on the merged tree (PEM banners in test sources)

- `packages/schema/src/domains/vpn.test.ts:41`, `:282`; `packages/schema/src/semantic/vpn.test.ts:364` contain the literal
  the PEM private-key banners (`BEGIN PRIVATE KEY` / `BEGIN RSA PRIVATE KEY` between five-dash fences) [quoted form redacted by P02c so the gate's private-key scan passes].
- `tools/ci.sh:370-372` (merged with P09 at `9919acf`, 15:26:58) greps committed/working files for `-----BEGIN [A-Z ]*PRIVATE KEY-----`
  and fails the gate. The worker's two pasted `CI GATE PASSED` runs (15:24:29 and 15:29:12, `/root/ngfw-wt/logs/P02c-ci*.log`) were made
  in the worktree without merging main, i.e. against the pre-P09 `ci.sh` (base `2de6c2f` has no such scan — verified). The paste is honest
  but no longer matches the gate the branch will be merged under; the repo's pre-merge hook runs `tools/ci.sh quick` and will refuse the merge.
- Failure scenario: `git merge task/P02c` on main → hook refuses; if merged with `--no-verify`, every later branch fails the gate on main.
- Fix (3 lines, no behaviour change): build the banners at runtime so the source never contains the pattern, e.g.
  `const PEM = ['-----BEGIN', 'PRIVATE KEY-----'].join(' ')` / `['-----BEGIN RSA', 'PRIVATE KEY-----'].join(' ')`, and use `PEM` in the
  three places. Then re-run `tools/ci.sh --base main` after merging main and paste the new output into `P02c.md`.

### F2 — BLOCKING (merge-time): exported symbols collide with P02a — and D-037 names the wrong symbol

- `packages/schema/src/domains/services.ts:526 DnsSchema`, `:817 NtpServerSchema`, `:841 NtpSchema` are also exported by
  `task/P02a:packages/schema/src/domains/system.ts:51 / :27 / :37`. `index.ts:54-66` re-exports both files with `export *` → TS2308
  ("already exported a member named …") on whichever of P02a/P02c merges second; `tools/ci.sh` typecheck fails.
- `docs/status/tasks/P02c.md` (decision P02c-2), `P02c-questions.md` Q2 and the contract doc claim the local names "do not collide"; the
  worker checked P02a's `primitives.ts` (`secretRef`, `hostOrIp`, `mtu`) but not its `system.ts`. LOG `D-037` records the hazard as
  `secretRef` — the branch exports `secretReference`, so D-037 is stale; the real collisions are the three above (the other joins in my
  export diff are the P02s placeholder exports of the same file and vanish on merge). `git merge-tree task/P02c task/P02b` is clean.
- Fix: rename in `services.ts` (e.g. `DnsServiceSchema`/`UnboundSchema`, `ChronySchema`/`NtpServiceSchema`, `NtpSourceSchema`) or resolve F3
  by removing one NTP model; update D-037 with the correct list. Owner: P02c (or the manager at merge, per D-037 — then this list is the input).

### F3 — MAJOR (decide before the `contracts-v1` tag): NTP is modelled twice

- P02a `system.ntp` (`NtpSchema` with `servers[]`, `system.ts:37-49`, prefault at `:83`) and P02c `services.ntp` (`services.ts:841-884`:
  servers, pools, allow, listen, localStratum, NTS/keyRef) both drive chrony's client side. Two sources of truth for one daemon; dropping one
  after the tag is a removal (breaking, decision-policy #1). The worker raised it (Q3) — correct — but it is unresolved and both branches are
  in review now.
- Recommendation: `services.ntp` is the more complete chrony model (server mode needs `allow`/`listen`/`localStratum`, D7.4 is a group-(c)
  row); keep it and drop `system.ntp` from P02a, or reduce P02a's to a `system.ntp.useServices: true` alias. Either way, one place.
  Similarly document that `system.dns` (P02a, management-host resolver client) and `services.dns.resolvers` (Unbound instances) are
  intentionally different objects — and rename the P02c export (F2).

### F4 — MAJOR (forces a rename after the tag): IPsec `vrf` means "underlay", everywhere else `vrf` means "overlay"

- `domains/vpn.ts:364 vrf: vrfRef` is the only VRF on `IpsecTunnelSchema`; `semantic/vpn.ts:126-127` checks `localAddr` in `t.vrf` and
  `semantic/vpn.ts:170` requires `ipip.underlayVrf === t.vrf`. So for IPsec `vrf` = the VRF IKE/ESP run in. For `tunnels.*` (`tunnels.ts:23-46`)
  and WireGuard (`vpn.ts:472-477`) `vrf` = the tunnel interface's FIB and `underlayVrf` = where `src`/`listenAddress` live. Probe m: a
  route-based tunnel whose IPIP has `vrf: customer-a` (overlay) and IPsec `vrf: default` validates with 0 findings — the IPsec object cannot
  say which overlay it serves, and a policy-based tunnel cannot say which VRF its traffic selectors apply to.
- Fixing this later means giving `vrf` a new meaning and adding `underlayVrf` — a reshape (always-PENDING). Do it now: add
  `underlayVrf: objectName.default('default')` to `IpsecTunnelSchema` and `RemoteAccessProfileSchema`; `localAddr` configured in `underlayVrf`;
  route-based rule: `ipip.underlayVrf === t.underlayVrf` **and** `ipip.vrf === t.vrf`; `vpn.vrf-exists` checks both; contract doc and P11 §2
  stay compatible (`vrf` remains, one additive field). `vpn.ipsec-peer-unique` should key on `underlayVrf`.

### F5 — MAJOR: default-free domain roots (P02c-1) contradict D-017 and are inconsistent with groups (a)/(b)

- `vpn.ts:703-718`, `tunnels.ts:232-252`, `services.ts:890-898`, `ha.ts:206-214`: every sub-tree is `.optional()` without a default.
  Probe n: `RootConfig.parse({}).vpn.ipsec === undefined`; the contract doc tells consumers to write `config.vpn.ipsec?.tunnels ?? {}`.
  The stated reason (main's `index.test.ts:26` asserting `{}`) is gone since D-036 (main now asserts "object"), and P02b's `NatSchema` /
  P02a's `SystemSchema` fill root-level defaults (`system.ntp: NtpSchema.prefault({})`). Two conventions in one document; P06 (commit
  engine), P07 (SchemaForm) and P11/renderers will special-case group (c).
- Adding the defaults later is additive in JSON-Schema terms but changes the normalised form of every stored revision (diff noise on the
  first commit after the change) and the TS types — cheapest now. Fix: `.prefault({})` on `ipsec`, `wireguard`, `pki`, `dhcp`, `dns`,
  `snmp`, `lldp`, `ipfix`, `ntp`; `.default({})` on `remoteAccess`, `gre`, `vxlan`, `ipip`; `.default([])` on `vrrp`; keep `cluster`,
  `pki.hsm`, `dns.vppCache`, `ipfix.sflow` optional (they have required fields). Update the four root tests and the contract doc.

### F6 — MEDIUM: `vpn.ipsec-peer-unique` rejects valid multi-responder setups

- `semantic/vpn.ts:211` keys on `vrf|localAddr|remoteAddr` including `%any`. Probe a: a second responder-only connection on the same local
  address (different PSK / `localId`, e.g. PSK spokes + certificate spokes) → `/vpn/ipsec/tunnels/hub-any-cert/remoteAddr … already uses
  198.51.100.2 → %any`. strongSwan selects among several `%any` connections by IDs/auth; this is a common hub design.
- Fix: exclude `%any` from the key, or key on `(underlayVrf, localAddr, remoteAddr, localId ?? '', remoteId ?? '', auth.method)`.

### F7 — MEDIUM: the "no inline secret" guarantee is weaker than the docs say; make `secretRef` carry a kind

- `vpn.ts:36-50 secretReference` accepts any `[A-Za-z0-9][A-Za-z0-9_.-]{0,62}`: probe c — `MyS3cretPSK2026`, `correct-horse-battery-staple`
  and a 32-hex string all pass schema **and** `vpn.no-inline-secret-material` (0 findings). Probe i: the scan (`semantic/vpn.ts:327,333`)
  misses a 16-byte base64 key, a 64-hex key and `-----BEGIN PGP PRIVATE KEY BLOCK-----`. The schema alone cannot enforce rule #10; the
  real guard is the API tier rejecting unknown refs — neither the contract doc nor P02c.md says so.
- Fix (do now, a later tightening is a pattern change = reshape): require the `<kind>/<name>` form with an enumerated kind
  (`psk|wg-key|wg-psk|x509|x509-key|x509-ca|snmp-community|snmp-auth|snmp-priv|eap|radius|ntp-key|cluster|pin`), which turns nearly every
  pasted secret into a schema error and gives `POST /api/v1/secrets` a typed `kind` (docs/04 `secret(kind, ref)`); broaden the PEM regex to
  `-----BEGIN [A-Z ]+-----` (no PEM of any kind belongs in config); state in the contract doc that ref existence is enforced by the API.
- Test coverage: `semantic/vpn.test.ts:361-412` plants strings only in `vpn` and `tunnels`; the `services`/`ha` walk is untested (probe h
  shows it works: `/ha/vrrp/0/description`, `/services/snmp/sysContact`). Add those two cases plus a RADIUS `secretRef`/SNMPv3 `authRef`
  negative so the "group-wide" claim is pinned by a test.

### F8 — MEDIUM: group-(c) helper primitives are public package API at tag time; the planned dedupe would be breaking

- `secretReference`, `transportPort`, `hostOrIpAddress`, `ipv4OrIpv6Cidr`, `wireguardKey`, `descriptionField`, `enabledFlag`, `vrfRef`,
  `u32Int`, `mtuField` (`vpn.ts:36-109`) and `dnsName` (`services.ts:341`) are `export const` in domain files that `index.ts` re-exports
  with `export *`. The worker's plan (Q2) to "re-point them at P02a's primitives" after the tag means deleting exports → breaking TS API.
- Fix: move them into a module `index.ts` does not re-export (e.g. `src/domains/group-c-common.ts` next to `semantic/tunnels-common.ts`),
  import them from there in the four domain files, export only schemas/types/constants. Then the dedupe is invisible. Same for the IP math
  (`semantic/tunnels-common.ts` is already not re-exported — fine; P02a has `src/ip.ts`, so one of the two goes later, internally).

### F9 — MEDIUM (manager decision): `ha.vrrp` as an array

- `ha.ts:208`: a virtual router has a natural identity `(interface, addressFamily, vrId)` but is addressed by index (`/ha/vrrp/3/vrId`);
  RFC 7386 merge-patch (`PATCH /api/v1/config/ha/vrrp/…`) cannot touch one element, D-021 makes arrays diff leaves (one VR edit = whole
  array replaced), and vdom.md #2's `/config/<domain>/<name>` has no `<name>`. docs/04 sketches `"vrrp": [...]`, so the worker followed the
  doc; every other group-(c) collection is a record keyed by name. Array → record after the tag is a reshape.
- Recommendation: `vrrp: record(objectName, VrrpInstanceSchema)` now; `ha.vrrp-vrid-unique` keeps the (interface, family, vrId) rule.
  If the manager keeps the array, say so in the contract doc so P06/P07 do not assume name-addressability.

### F10 — MEDIUM: cross-domain address overlap is nobody's rule

- `semantic/tunnels.ts:167-192` compares tunnel prefixes only with other tunnels. Probe k (`gre-dc.ipv4 = 192.168.10.77/24` while
  `TenGigabitEthernet0/0/1` has `192.168.10.1/24`, same VRF) and probe l (WireGuard `address` overlapping a physical prefix) → 0 findings.
  P02a's "no overlapping IPv4 on the same VRF" covers `interfaces` only.
- Fix: `tunnels.address-overlap` already builds `interfaceIndex(config)` — seed `seen` with the prefixes of `interfaces` (+ sub-interfaces)
  and WireGuard `address[]`, and add the same check for WireGuard in `vpn.wireguard-unique`. Additive.

### F11 — MEDIUM/LOW: `tunnels.endpoints-unique` false positive for GRE

- `semantic/tunnels.ts:111` keys GRE on `(underlayVrf, src, dst)`. Probe j: two ERSPAN tunnels with different `sessionId` to the same
  collector → `/tunnels/gre/mirror2/dst same endpoints as tunnel 'mirror'`. VPP keys GRE tunnels on type/session/mode as well.
- Fix: include `type` and `sessionId ?? ''` in the GRE key.

### F12 — LOW: a mistyped IP is accepted as a hostname

- `vpn.ts:332 remoteAddr: z.union([ipAddress, hostname, '%any'])`, `vpn.ts:56 hostOrIpAddress`: probe b — `203.0.113.999` is accepted
  (RFC 1123 allows all-digit labels) and fails only at apply time (DNS). Fix: refine the hostname branch with `!/^[0-9.]+$/.test(v)`.

### F13 — LOW: DHCP relay `sourceAddress` VRF

- `services.ts:308-311` + `semantic/services.ts:153-155` require `sourceAddress` in the client `vrf`. VPP `dhcp_proxy_config`
  (`apps/agent/binapi/dhcp`) uses `dhcp_src_address` as the source towards `dhcp_server` in `server_vrf_id`. Probe o: `serverVrf: default`,
  `sourceAddress` = an address in `default` → rejected. Fix: accept an address configured in `serverVrf ?? vrf` (or either), and say which.

### F14 — LOW: no cross-check proposal ↔ `protocol`/`ikeVersion`

- Probe d: `protocol: 'ah'` with an encrypting ESP proposal, and `ikeVersion: 1` with `chacha20poly1305`/`curve25519`, both validate.
  Additive semantic rule later; note it in the contract doc for P11.

### F15 — LOW: singletons that may want to become records

- `services.snmp`, `services.ntp`, `services.lldp`, `services.ipfix.flowprobe`, `services.ipfix.sflow` are single objects with one `vrf`.
  LLDP/flowprobe/sFlow are global in VPP — fine. SNMP/NTP per-VRF instances later would be a reshape; acceptable for management-plane
  daemons if the contract doc states "one instance, bound to `vrf`" as the v1 decision.

### F16 — LOW (process)

- No `docs/status/tasks/P02c-contract.md` (00-CONTEXT "labelled contract" convention); `P02c.md` covers the content — manager may waive.
- `docs/status/tasks/P02c.md` still says the branch base is `main@2de6c2f`; after F1 the status must show a gate run on a tree that
  includes P09's `ci.sh`.
- `semantic/tunnels-common.ts`: acceptable — pure, not re-exported, duck-typed against documented field names, matches P02a's actual
  `interfaces` shape (`ipv4[]`, `ipv6[]`, `vrf`, `subinterfaces{vlanId}` — verified on `task/P02a`), 10 tests. The layering
  (`domains/services.ts` importing from `semantic/`) is a smell only; move the IP math to `src/ip.ts` when P02a's lands.

## WBS coverage (item 1) — what the contract can express

| WBS | status | note |
|---|---|---|
| D6.1 IPsec core | partial | engine/async flags; manually-keyed SAs / SPD-only entries not modelled (additive `vpn.ipsec.manualSas{}`) |
| D6.2 strongSwan (P11 §2) | yes | `proposals{ike,esp}`, `tunnels{…, auth{psk(secretRef)\|cert}, routeBased{ipipInterface}}`, IKEv1/2, DPD, NAT-T, MOBIKE, DH 1–2,5,14–26,31,32 |
| D6.3 native IKEv2 | yes (flag) | `engine: vpp-ikev2`, IKEv2-only enforced |
| D6.4 PKI | yes (refs) | CA/cert/CRL/OCSP/ACME/HSM as references |
| D6.5 WireGuard | yes | public key validated, private/PSK as refs |
| D6.6 tunnels | partial | GRE l3/teb/erspan, VXLAN (mcast, decap), IPIP p2p/p2mp; **VXLAN-GPE, GTP-U, L2TPv3, PPPoE absent** (additive keys) |
| D6.7 SRv6 / D6.8 LISP | no | additive later; decide the home (`routing.srv6` is P02a's file) |
| D6.9 RA-VPN | yes | IKEv2+EAP, pools, RADIUS refs |
| D7.1 Kea | yes | instances with `vrf`+`interfaces` (vdom #5) |
| D7.2 relay / **DHCP client** | relay yes; **client has no home** (neither here nor in P02a's `interfaces`) | cross-group gap — decide (`interfaces[].dhcpClient`) |
| D7.3 Unbound / VPP dns | yes | views absent (additive) |
| D7.4 chrony | yes | but duplicated with `system.ntp` (F3); PTP absent |
| D7.5 SNMP | yes | communities/USM keys as refs |
| D7.6 IPFIX/sFlow | yes | |
| D7.7 syslog | n/a | P02a `management.syslog` (verified) |
| **D7.8 QoS** | **no** | policer/shaper/marking/HQoS have no key anywhere; reserve `services.qos` (or decide a root key now — a new root key touches `ROOT_KEYS`) |
| D7.9 LB / D7.10 host stack | no | T3; additive `services.lb`, `services.hostStack` |
| D7.11 PG | n/a | actions/state |
| D9.1 VRRP | yes | VRRPv3 semantics, vpp\|keepalived engine; array shape → F9 |
| D9.2/D9.3 cluster | yes (minimal) | membership, config sync, state-sync flags, key as ref |

All gaps are additive (new optional keys under strict roots) except where noted (F4, F5, F7, F9 are the ones that get expensive after the tag).

## Verified OK

- Secrets: every secret-bearing field is a `*Ref` (`secretReference`); strict objects reject `psk`, `password`, `privateKey`, `presharedKey`,
  `community`, `authPassword`, `secret`, `key` (tested per domain); WireGuard public keys validated as exactly 32 bytes; fixtures use
  `VRX_TEST_PSK_inline` and the public WireGuard docs keys only. Group-wide scan exists (`vpn.no-inline-secret-material`) and reaches all
  four domains (probe h) — with the limits in F7.
- vdom guardrails: `vrf` on every IPsec/WireGuard/tunnel/service/VR object (+`underlayVrf` on tunnels/WireGuard; F4 for IPsec); names unique
  per record and across tunnel kinds; Kea/Unbound are instance records with their own bind sets; `default` VRF always known.
- Pointers: built with `jsonPointer()`; record keys are `objectName` (no `/`), so no escaping traps; four fixtures verified above; Zod
  `unrecognized_keys` handled as `<object>/<key>` in tests (Q8 is a real note for P06).
- Hostile inputs: NUL/LF, unicode, over-long, wrong padding, 43/45-char keys, `%any`+start, mixed families, multicast/unspecified endpoints,
  VNI 2^24, VRID 0/256, 15 ms interval, pools outside subnets, case/separator-insensitive MAC duplicates, Kea/Unbound passthrough keys, loops.
- `x-vrx-ui`: `withUi()` on the four roots (`order` 90/100/110/120), sub-trees (`group`/`order`) and fields (`widget` select/switch/number/
  textarea/cidr/record/vrf-picker/interface-picker/secret-ref, `help`); verified in `dist/json-schema/{ha,vpn}.json`.
- VRID unique per (interface, addressFamily) — correct (RFC 5798; VPP `vrrp_vr_add_del` keys on sw_if_index + is_ipv6 + vr_id). Accept P02c-4.
- Interface references by VPP name; tunnels referenceable only with `instance` — sane, documented (P02c-7).

## Approval condition

Fix F1 and F2 on the branch (re-run `tools/ci.sh --base main` after merging main, paste the output), and record a decision on F3 before the
`contracts-v1` tag; F4, F5, F7, F9 are the shape decisions that become breaking after the tag — decide them now or log why not.

**BLOCK**
