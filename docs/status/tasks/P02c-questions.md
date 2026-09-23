# P02c — questions for the manager (none blocking; work continued with the stated choice)

1. **`index.test.ts` vs D-017 nested defaults.** `index.test.ts:26` asserts `RootConfig.parse({})[key]` deep-equals `{}` for every root key,
   which fails as soon as a domain root carries a `.default`/`.prefault` (P02b's `NatSchema` has root-level defaults and will hit it too).
   D-017's stated intent ("prefault fills nested defaults on parse") points the other way. P02a owns the test. My choice: the four group-(c)
   roots are default-free (sub-trees optional, defaults inside), so the branch passes CI standalone. If P02a relaxes the test (e.g.
   `expect(r[key]).toEqual(DOMAINS[key].parse({}))`), adding `.prefault({})` on my sub-trees later is additive.
2. **Local primitives duplicate P02a/P02b work.** `task/P02a` exports `secretRef`, `hostOrIp`, `mtu`, `portNumber`, `ipCidr` from
   `primitives.ts` and has `src/ip.ts`; `task/P02b` exports `l4Port`. Because `index.ts` re-exports every domain file with `export *`, my
   copies use distinct names (`secretReference`, `transportPort`, `hostOrIpAddress`, `ipv4OrIpv6Cidr`, `mtuField`, `vrfRef`,
   `descriptionField`, `enabledFlag`, `u32Int`, `wireguardKey`; IP math in `semantic/tunnels-common.ts`). After both merge, a follow-up
   `contract(schema)` should re-point them at the shared primitives (JSON Schema unchanged; field names untouched). Candidates for
   `primitives.ts`: `secretReference` (regex forbids `+`, `=`, spaces, PEM), `wireguardKey`, `dnsName`, `transportPort`.
3. **`services.ntp` vs docs/04 `system.ntp` / `system.dns`.** docs/04 puts `ntp`/`dns` client settings under `system` (P02a); WBS D7.4
   (chrony client/server) is in group (c). I modelled the chrony daemon as `services.ntp` (servers, pools, allow, listen, local stratum,
   NTS/key refs) and Unbound as `services.dns.resolvers`. Proposal: `system.ntp`/`system.dns` (if P02a models them) stay the management-host
   client settings; if the manager prefers a single place, dropping one before `contracts-v1` is trivial. Please decide.
4. **Interface-reference convention.** Group-(c) objects reference interfaces by VPP name (`vppInterfaceName`), resolved against
   `interfaces` (+ sub-interfaces `<parent>.<key>`/`<parent>.<vlanId>`), tunnels with an explicit `instance` (`gre<n>`, `ipip<n>`,
   `vxlan_tunnel<n>`) and WireGuard `wg<instance>`. IPsec `routeBased.ipipInterface` references the *object name* in `tunnels.ipip` (P11 §2).
   Confirm P02a's sub-interface key convention (two-interfaces.json uses key `"100"` with `vlanId: 100`; both spellings are accepted here).
5. **Scope beyond the envelope's WBS list.** `vpn.pki` (D6.4) and `vpn.remoteAccess` (D6.9) were in the recovered draft and are needed for
   P11's `auth{cert}` shape; kept minimal (references only, no material). `services.ipfix`/`sflow` (D7.6) and `ha.cluster` (D9.2/D9.3)
   follow docs/04's key list. Say if any should be cut before the tag.
6. **Branch coverage.** `@vitest/coverage-v8` lives on `task/P02a` (D-024); it is not on main, so I could not measure the 100 %-branch
   acceptance item here. Run `pnpm -C packages/schema test:coverage` after P02a merges; the semantic files have one test per branch by design.
7. **VRID uniqueness** is per `(interface, addressFamily)`, not per interface alone (RFC 5798 and VPP `vrrp_vr_add_del` key on
   `sw_if_index + is_ipv6 + vr_id`; keepalived allows it too). Tightening to per-interface would reject valid dual-stack setups — say if wanted.
8. **Zod 4 unknown-key issues** carry `path: []` and `keys: [...]` (`code: 'unrecognized_keys'`). API error mapping (P06) should render the
   pointer as `<object pointer>/<key>`; my tests do the same.

## Status after the fix round (2026-09-24)

- Q1 resolved by D-036 + D-053 (roots prefault now). Q2 resolved by D-047/D-054 (`Services*` names; helpers private in
  `domains/_shared/primitives.ts`, so the dedupe with P02a's `primitives.ts` is internal). Q3 resolved by D-050 (`services.ntp` only).
- Q6 still open: `@vitest/coverage-v8` is not on main; per D-048 each group adds its own threshold later. Not measured here.
- Q9 (new) — **`apps/agent` / `packages/proto/test` literals.** The D-053 `ha.vrrp` reshape forced two one-line edits outside my file set
  (`apps/agent/internal/contracttest/desiredstate_test.go:472`, `packages/proto/test/desired-state.test.ts:236`: `vrrp: [{}]` →
  `vrrp: { lan: {} }`). Needed for a green gate; please confirm at merge.
- Q10 (new) — **History rewrite.** `tools/ci.sh` runs gitleaks over `main..HEAD`; commits of the first round (`a306c2b`: WireGuard
  public keys / an inline-key test value flagged as generic-api-key) and the review commit (`4cf4236`: a quoted PEM banner) failed it,
  and ci.sh says "recreate the commits without it". I rebuilt the branch as fresh commits on top of main (old tip `f8b569a`, reachable
  via reflog). The review text is preserved in `P02c-review.md` with the banner quote redacted.
- Q11 (new) — **P02a follow-ups** (their files, not mine): drop `system.ntp` (D-050), rename `DnsSchema`/`NtpSchema`/`NtpServerSchema` to
  `System*` (D-047) — until then `export *` collides only if P02a still exports those names; my side exports none of them. `secretRef` on
  P02a's `primitives.ts` still accepts free strings (`ipsec/psk/site-a`); D-051 asks for `<kind>/<name>` there too.
- Q12 (new) — **F14 scope.** `vpn.proposal-compatible` rejects AEAD IKE ciphers and Curve25519/448 for IKEv1 (RFC 5282 / RFC 8031 are
  IKEv2-only) and encrypting proposals for AH. If strongSwan's IKEv1 GCM extension should be allowed, relax the first rule.
