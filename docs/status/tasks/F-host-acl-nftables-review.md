# Review — F-host-acl-nftables (task/F-host-acl-nftables @ e5e21afc; code 942f292d, contract 3fdb302f)

Reviewer, 2026-09-25. Scope: this task's commits `31249d33..e5e21afc`. The rest of the branch is the unsquashed
P08 / W-seed / F-object-model base.

## Verdict: **APPROVE WITH CHANGES**

The architecture is right. The renderer is a single declarative descriptor (`host-acl.nftables/vrx`), and the agent
core has no imperative path. Only the agent touches nftables, and Node reads through `HostAclState`. One schema serves
all three consumers. The injection surface is closed by construction. Mode `check` never writes to the root netns.
Tests and evidence reproduce, including my own re-run of the netns test on slot 9.

Two problems must be fixed before the merge:
- **H1:** the product's anti-lockout guarantee can still be broken with default settings. An output list that drops
  traffic, attached at priority ≤ −200, runs before conntrack.
- **H2:** as soon as `tools/app` is rebuilt, the product stack on this shared host would load `table inet vrx` into the
  root netns.

Both fixes are small. A focused re-verify of H1, H2, M1 and M4 is enough; a full re-review is not needed.

## What I ran

| check | result |
|---|---|
| `go test -race -count=1 ./internal/renderers/nftables/... ./internal/subsystems/... ./internal/agent/... ./internal/desired/...` | `ok` for all four packages. The descriptor lives in `renderers/nftables`, so it is covered |
| `pnpm --filter @ngfw/web test` (deps built with turbo) | 18 files, 116 tests passed. Includes `host-acl-nftables/model.test.ts` (9) and `HostAclPage.test.tsx` (3) |
| API module `vitest run src/features/host-acl-nftables` | 4/4 passed |
| schema `vitest run src/semantic/host-acl-nftables.test.ts` | 4/4 passed |
| `VRX_INTEGRATION=1 … -run TestIntegrationHostFirewallInSlotNetns` on slot 9, under `tools/lab lock shared`, run once | PASS (2.71 s). Rules listed in `ns-w9-hacl`. Port 2323 got an i/o timeout and its counter went 0 → 2. 2222 connected. 2424 was refused after the update. Rollback listing identical. Restart re-render 159 ms with an empty diff. Removal left only `inet foreign` |
| root netns `nft list tables`, before and after my run | identical: `ip filter`, `ip nat`, `ip mangle`, `ip6 filter`, `ip6 nat`, `ip6 mangle`. `nftables.service` inactive and disabled both times. Afterwards: no w9 netns or links, and the empty `/run/vrx-test/w9` was removed |
| probes (a scratch copy of the module, nothing added to the worktree) | the results are H1, M1 and the L items below |
| worktree after the runs | `git status` clean. The `dist/` directories I created were removed |

The worker's pasted evidence is complete and consistent with my re-run: allowed and dropped ports, counters,
rollback via the API with listing and Retrieve, restart re-render, and the 400 with pointer `/acl/host/local-in/rules/0`.

## Findings

| id | sev | where | finding | fix |
|---|---|---|---|---|
| H1 | **H** | `lockout.go:110-113` (only `input` chains are simulated); `build.go:99-117` (any priority −500…500 on every hook); `docs/user/firewall/host-acl-nftables.md:26,38` | **A commit can lock out management, and neither the rule nor the check stops it.** On the output hook, conntrack runs at priority −200 (`NF_IP_PRI_CONNTRACK`). A chain at −300 sees locally generated packets before they have a ct entry: `ct state established,related` does not match. The SSH/HTTPS replies (SYN-ACK and every data segment of the operator's own session) therefore fall through to the user's rules. Probe `{"host":{"eg":{"rules":[{"sequence":1,"action":"drop"}]}},"hostAttachments":[{"list":"eg","chain":"output","priority":-300}]}` builds with **no issue**, both with anti-lockout on (the default) and off, and renders `chain out_eg { type filter hook output priority -300; … ct state established,related … accept; oif "lo" … accept; counter drop }`. The anti-lockout rule exists only in input chains. The user doc's "Every chain starts with established/related … replies … are never cut" and "A commit can never cut the management connection" are false in this case | Smallest fix: `Build` refuses an `output` attachment with priority ≤ −200, with an error at `/acl/hostAttachments/<i>/priority` ("output chains must run after conntrack"). Add the same check as a tier-b semantic rule for early feedback. Alternative: also render an anti-lockout rule in output chains (`tcp sport {ports}` to the sources) and simulate reply probes. Add a lockout test case and correct the docs |
| H2 | **H** (merge precondition) | `paths.go:71-86,101-129`; `subsystems/host_acl.go:29`; `tools/app:108` (owner `vrx`, `VRX_GLOBALS_OWNER=0`, no `VRX_HOST_ACL_MODE`) | Once merged and rebuilt, `tools/app`'s agent (owner `vrx`) runs in mode **apply** on the shared host's root netns. The first commit of `acl.hostAttachments` through the dev UI then loads `table inet vrx`. With `defaultInput: drop` that cuts the browser from 3000/8080; only 22 and 443 stay open. This is the Q5 hazard | Make it structural, not a `tools/app` convention. The root-netns firewall is a host-wide singleton, which is D-071's globals concept. Mode `apply` should require owner `vrx` **and** `Env.GlobalsOwner`, so the default is `check` when `GlobalsOwner` is false, and `VRX_HOST_ACL_MODE=apply` should be refused when `VRX_GLOBALS_OWNER=0`. That is about five lines plus one `TestPathsFromEnv` case. `tools/app` already sets `VRX_GLOBALS_OWNER=0`, and a real box defaults to true. Minimum alternative: the manager adds `VRX_HOST_ACL_MODE=check` to `tools/app:108` **before** this branch merges |
| M1 | M | `descriptor.go:159-173` (`annotate`); `parse.go:46-107`; `docs/user/firewall/host-acl-nftables.md:125` | **Drift detection has blind spots, and one of them hides a security change.** `annotate` overwrites the kernel's parsed verdict with the stored one whenever the comment matches, so a hand edit that flips `drop` → `accept` and keeps the comment makes Retrieve equal desired. The probe on `kernel-full.json` gives `visible=false`, and a port edit gives `visible=false` too. The table object is never parsed, so `nft add table inet vrx { flags dormant; }`, which disables the whole firewall, is also invisible (`visible=false`). Additions, removals, reorders, set elements, policy, priority and comment edits are detected (`TestKernelDriftIsVisible`). The user doc promises that any hand change is put back | (a) In `annotate`, skip a rule whose kernel verdict differs from the stored one. That is one condition, and the rule then differs, which causes an Update. (b) Parse the table's `flags` and treat `dormant` as drift. (c) Optional, closes body edits: after `nft -f`, store a hash of each rule's kernel `expr` JSON (counters stripped) and compare it on Retrieve. This needs no prediction of nft's re-printing. Qualify the doc sentence |
| M2 | M | `lockout.go:184-198` + `subsystems/host_acl.go:40` + `agent/service.go:344-348` | **Runtime answers can make a resync fail.** After the rebase, TD-8's `Env.Resync` is live, so an FQDN answer change starts a full resync. Take anti-lockout off, `defaultInput: drop`, and a management accept whose source is an FQDN object. When the answer changes, or when D-129 F7 drops the last-good answer after 24 h, `Build` reports `acl.host-anti-lockout` at `/acl/hostSettings/defaultInput`. My probe confirms that an unresolved FQDN gives this error. `applyLocked` then applies **nothing** for any domain in that resync (`service.go:344`). The table stays safe, but VPP reconciliation, for example after a reconnect, is coupled to DNS | In the lockout simulation, let an accept whose source or destination is an FQDN-bearing object not count as protecting management (`relNone`). Such a config is then refused at commit time and can never fail later. Alternatively, record it as tech debt for TD-13 (mode-aware findings) |
| M3 | M | `agent/service_test.go:600` | Q7: the hard-coded example `"vpn"` becomes implemented when F-wireguard merges (`VPN = "vpn"` on task/F-wireguard). F-acl's `"management"` becomes implemented when F-unbound-chrony-syslog merges. Each word breaks a later merge gate | Derive it from the registry, in the D-129 F5 spirit: take the first `rootKeys` entry with no `subsystems.Domains` entry and skip the assertion if there is none. It is test-only; do it in this fix round so the F-acl merge stays mechanical (D-134) |
| M4 | M (programme, not this branch) | `prompts/P10-packaging-deb.md:51-53`; `docs/user/firewall/host-acl-nftables.md:13-14` | **Architecture note for the manager.** P10's `inet vrx_base` is planned as "allow 22/443 on mgmt, drop everything else inbound". In nftables a packet must pass *every* base chain on a hook, so a host-ACL accept for SNMP, BGP, DNS or NTP is still dropped by `vrx_base`. The prompt's "base policy becomes its bootstrap default" cannot be achieved with two independent tables, and the anti-lockout simulation cannot see `vrx_base` either. The same applies on this dev host to the iptables-nft `ip filter` tables | A manager decision before P10: for example `vrx_base` with policy accept and only invariant drops; or the base policy shipped as the initial content of `inet vrx`, which the agent replaces; or the agent deletes `vrx_base` on its first render. In this branch, only add one sentence to the user doc: other tables can still drop what this ACL accepts. Defence in depth: `tableRe` (`paths.go:51`) could exclude `vrx_base` |
| L1 | L | `lockout.go:93`, `lockout.go:54-72` | With anti-lockout off and no `sources`, any drop on 22 or 443 is refused, even from a single bad /32 (probe: `198.51.100.7/32 tcp/22` → error). This is conservative by design, but the message's advice "narrow this one" cannot help | Add "or set `antiLockout.sources` to your management network" to the message |
| L2 | L | `build.go:536-566` | IPv4-only `antiLockout.sources` with `defaultInput: drop` silently leaves IPv6 management unprotected. This follows the configuration, but nothing warns | A warning when the sources cover one family and `defaultInput` is drop |
| L3 | L | `build.go:407` | `log` has no rate limit. A logged drop-all under a scan floods the kernel log and journal | `limit rate 10/second burst 20 packets log …`, or document it |
| L4 | L | `docs/user/firewall/host-acl-nftables.md:38` | Recommend a confirmed commit (`?confirm=<sec>`, AD-4 §5) for every host-ACL change. It is the backstop for what the simulation cannot see: other tables, a wrong `sources` for the operator's own address, H1-like cases | One paragraph |
| L5 | L | `web/…/queries.ts:12` | A stale doc comment ("a 5 s poll") sits above the D-132 comment | Delete it |
| L6 | L | `web/…/model.ts:54,57`; `schema/…/ext/host-acl-nftables.ts:19` | `LIST_NAME_RE`, `DEFAULT_SETTINGS` and `hostInterfaceName` are hand copies of schema facts. The last one avoids an import cycle, which is fine but should say so | Derive `DEFAULT_SETTINGS` from `HostAclSettingsSchema.parse({})` and the list-name pattern from the JSON Schema. Add a comment explaining the cycle |

Checked and fine:
- **Safety of the rendered text.** Nothing ever touches a table other than `inet vrx`/`vrx_<prefix>`. `RenderText` emits only `add table`/`delete table`/`table {…}` with a regex-checked name. Every rule text is re-checked (no `;`, `#`, `\`, control characters, flat braces), and everything that reaches the text is re-serialised from parsed `netip` values or regex-limited names. Hostile descriptions are never rendered.
- **Load order.** `nft -c` on a staged copy always runs before the atomic `nft -f`, and a failed load restores the file.
- **Netns runner.** `SystemRunner.Run` → `proc.Run()` forks on the locked, `setns`'d thread. A missing namespace fails closed.
- **Anti-lockout on the input hook** holds on every other vector I tried: ordering (the preamble comes first in every input chain), several input chains (`TestAntiLockoutAcrossChains`), interface-scoped rules (partial = conservative), IPv6 (a source-less rule covers both families; ND is always accepted), a missing established rule (it is always rendered) and an unresolved FQDN accept (error). Overlapping anti-lockout sources pass `nft -c`, because anonymous sets auto-merge.
- **TD-11b.** `RecordsNoOwnership()` is **correct**. Ownership is the exclusively owned table name. The store is only the applied value used for annotations, it is always a file, and losing it self-heals: the value differs, so an Update re-renders it. After the rebase the registration lands inside the guarded `register()`.
- **D-132.** 30 s poll plus a Refresh button. `HostAclState` reads `nft` and the store, never VPP.
- **UI.** 4 tabs. The en and fa key sets are identical (163 keys). Types come from the generated api-client and `@ngfw/schema`, and forms are `SchemaForm` over `domainSchemas`. No `dropPhantomOptionals`. No physical left/right CSS.

## Answers to the questions (the reviewer's recommendations)

- **Q1** — Allocate **`AclConfig.host_settings = 8`**. It matches wave-A-hotspots §2 and wave-BC-numbers:456 (7 = F-acl, 8 = host-acl, each only with a config gap). task/F-acl@9c3cbd2a adds no `AclConfig` field and no message name overlaps with the `HostAcl*` messages. The change is additive.
- **Q2** — Accept **(a)** and log it. When TD-13 lands, this descriptor should adopt TD-13's `Validator` (Render + `nft -c` at DryRun; today DryRun runs only `Build`) and the daemon stage.
- **Q3** — Accept "empty = any" with ports 22 and 443. It is the safe default, and the doc tells operators to narrow it. Later: default `interfaces` to the management port-group (D-026). Worth a tech-debt row: the API knows the committing client's source address and could warn when it is outside `antiLockout.sources` and no rule accepts it.
- **Q4** — Accept nft's default `reject` (icmpx port-unreachable; TCP clients see ECONNREFUSED, as in the evidence). No setting for now.
- **Q5** — Accepted, and confirmed: mode `check` **never writes to the root netns**. It runs only `nft -c -f <staged>`, which the kernel processes and then aborts. No `nft -f` runs (`renderer.go:99-101`). Retrieve and HostAclState do not even run `nft list` (`renderer.go:121-123`, `state.go:47-50`). Delete runs `nft -c` and then writes files. Files go only to the state dir. Make it structural, as in H2 (a `GlobalsOwner` gate). If it is not structural, `VRX_HOST_ACL_MODE=check` must be in `tools/app` before this merges.
- **Q6** — Resolved by the rebase. Main has TD-8's `Env.Resync` → `requestResync` with a storm guard, so `Wiring.RequestResync` on an FQDN change now re-renders. Re-verify once after the rebase, and see M2.
- **Q7** — Not fine as a fixed word: `"vpn"` breaks when F-wireguard merges. Derive it from the registry (M3).
- **Q8** — See the merge notes below.
- **Q9** — Confirmed. `git merge-tree --merge-base=31249d33 main task/F-host-acl-nftables`, which applies only this task's commits onto main, yields main's `test/topology/interfaces/interfaces_test.go`. A grep for `trace add|show trace|clear trace` finds nothing. After the rebase, run the gate with no pathspec exclusion.

## Merge notes

- **Conflicts.** The plain `git merge-tree --write-tree main task/F-host-acl-nftables` shows 34 conflicted files. They come from the unsquashed P08/W-seed/F-object-model base and are not this task's. Two simulations give the real picture:
  - Rebase simulation onto task/F-object-model's squashed tip 97316879 (`--merge-base=31249d33`): **1 conflict**, `apps/cli/internal/api/operations_gen.go`, which is generated → `make -C apps/cli gen docs`.
  - This task's own diff onto current main: **4 conflicts, all generated** (`dataplane.pb.go`, `dataplane_grpc.pb.go`, `packages/proto/gen/ts/…/dataplane.ts`, `operations_gen.go`) → `pnpm gen` plus the CLI generator. Every hotspot hunk auto-merges.
- **Order.** F-object-model merges first; it has its own `metrics.go` conflict with main. D-125 board gates: merge after TD-11a (escape hatch: APPROVE'd and waiting more than 2 h) and after **TD-13**, which has no escape hatch and is `ready` but not started. The manager must either keep that wait or move the TD-13 adoption into TD-13's obligations (Q2).
- **Squash.** Contract paths change: proto and schema in 3fdb302f, and the api-client generated file also in 18288752. The single squashed subject must therefore start with `contract(…)`, for example `contract(proto,schema,api-client): F-host-acl-nftables — …` (D-112).
- **With F-acl (Q8), whichever of the two merges second:**
  - `subsystems.go` const block: keep **one** `ACL = "acl"`, under `// wave-A: F-acl`, and drop the other. Two lines would be a duplicate const.
  - `Domains`: keep **one** entry, `ACL: append(aclDescriptors(), hostACLDescriptors()...)`. Two keys would be a duplicate map key.
  - `Register`/`register()`: keep both calls in anchor order: `registerObjectModel` → `registerACL` → `registerHostACL`.
  - `projection.go`: keep both `project` calls. The `assemble` order must stay F-acl **then** host-acl, because F-acl's `AssembleACL` overwrites `ds.Acl` while `AssembleHostACL` merges into it. Reversed, `/acl/host*` would drift forever.
  - Remove both "unsupported-field" blocks: F-acl `internal/desired/acl.go:95-99` (host leaves) and host-acl `internal/desired/hostacl.go:47-54` (F-acl leaves). This is a code change, so it belongs in the second branch's reviewed rebase commit, not in the merge (D-134).
  - `service_test.go:600`: the registry-derived form from M3.
  - proto, API, web and ALLOWLIST hunks sit under separate anchors.
