# P10 dynamic LCP punt-set implementation handoff

Design branch `task/P10-punt-design-current`, exact foundation `7f35978a`. Read-only investigation; no product/host mutations. P10 requires working punt paths in the packaging-owned base firewall; bootstrap CSV alone is not dynamic synchronization. This is implementation work, not an acceptance test deferred for missing lab access.

## Existing seams and failure

`deploy/debian/vrx/assets/render-base-policy.py:render` builds only `inet vrx_base`, an explicit `type ifname` set `punt_interfaces`, and separate management 22/443 rules; max64, unique safe names, management overlap rejected. `firewall-bootstrap.sh` saves nonsecret authoritative interface inputs in `/etc/vrx/base-policy.env`. No production agent code updates this set afterward. A new LCP pair is therefore dropped by the priority -10 base chain even if the independently owned `inet vrx` host-policy accepts it.

`subsystems/frr.go:registerP12` registers `tapGatedPairs`; its `Retrieve` delegates to owned `lcp.ItfPairDescriptor.Retrieve`. `lcp/lcp.go:Create` creates the Linux pair and persists ownership; `Delete` re-resolves ownership and removes it safely; Update returns ErrRecreate. `lcpmap.HostName` / FromDesired supply validated desired host names. `agent/service.go:applyLocked` and `resyncLocked` run the scheduler under existing transaction serialization. `scheduler.ApplyWith` owns rollback/verification. Use these seams rather than an eventually consistent post-commit watcher that could report APPLIED before firewall failure.

## Original singleton proposal — superseded by lifecycle correction below

Add a separate product-only scheduler singleton `base-policy.punt-set/vrx`, internal typed value containing canonical root-netns LCP host names (sorted, unique, bounded64). Derive it in interfaces projection from explicitly configured LCP leaves; do not expose a new public schema/RPC or wildcard. Register its renderer only for the appliance globals owner with explicit base-policy input configuration; shared test agents remain off, and absence of an expected product base table is an error rather than silently passing.

The descriptor owns ONLY `inet vrx_base` / `punt_interfaces`. Use existing `renderers.Runner` and fixed `/usr/sbin/nft` argv with typed stdin; never Python/shell trampolines. Render `flush set inet vrx_base punt_interfaces` plus optional `add element inet vrx_base punt_interfaces { quoted-safe-names }` in one nft transaction. Do not flush/recreate the table or ruleset, alter management rules, or modify `inet vrx`. Retrieve via bounded nft JSON parses actual elements and returns typed state; corrupted/wrong-type sets and missing table fail closed. No-op equality avoids writes; nft transaction rejection preserves old membership.

Dependencies are the relevant `lcp.itf-pair/<vppName>` keys, so create follows successful pair creation and reverse-delete order revokes membership before pair removal. Dependency recreation must clear the singleton membership (safe denial) then rebuild the remaining exact set, preserving management connectivity. Changes to pair names/types, removal, rollback and restart must run through this same scheduler ownership model. A failed sync fails the transaction and normal rollback restores the previous set and LCP pairs; verify compensation failures are DEGRADED, never APPLIED. Do not wrap pair Create with a best-effort background callback or return ordinary error after mutating a pair without a compensation journal.

Root-netns admission is critical: an explicitly named LCP Netns or a configured default LCP namespace means the tap is not in the host root namespace. Exclude such pairs using actual namespace semantics, not the desired empty string alone. Resolve default namespace from the owned singleton/VPP readback; reject ambiguous product namespace state. This cannot be inferred from lcpmap.FromDesired alone. Membership for another namespace must never admit an unrelated same-named host device.

Read `/etc/vrx/base-policy.env` as root-owned regular non-symlink trusted configuration with bounded parser, not shell sourcing. Management identity is immutable input to the sync and forbidden as a member. Explicit bootstrap punt entries are a separate permanent admin allowlist; decide/document union semantics and enforce the combined64 cap before mutation. Persisted nft bootstrap file should contain only permanent entries; dynamic desired membership is reconstructed by agent startup/resync after table loading, not written into broad `/etc` from the sandbox. Ensure boot ordering loads nftables/base policy before agent starts.

## Test handoff

Add pure renderer tests for names/escaping, duplicates, empty/max64/max65, management overlap, namespace collisions, bounded malformed JSON and foreign tables. Fake nft runner must model atomic set replacement and injected validation/apply failures. Scheduler integration tests must cover pair create/delete/name change, rollback after sync failure and later descriptor failure, DEGRADED compensation failure, unchanged-state restart restoring deleted set elements, foreign pairs excluded, unavailable plugin, named/default namespace handling, and disabled lab-agent no-call behavior.

Extend existing `TestItfPair`, `TestTapGatedPairs`, `TestFRRStageSurvivesPairChanges` rather than weakening them. Confirm FRR config is not recreated due to new dependencies: its current singleton deliberately has no LCP dependencies. Packaging unit tests should assert nftables/base-policy ordering and boot restoration; actual nft/VPP traffic and reboot remain NOT RUN until centralized campaign execution.

## Privileges and code scope

Authorized scope: P10 base firewall, agent internal typed adapter/projection/registration, narrow dependency/boot-order integration, tests and task docs. Existing CAP_NET_ADMIN plus AF_NETLINK suffice for nft set updates; CAP_SYS_ADMIN supports existing NETNS handling but this feature should not create extra namespaces. Unit hardening leaves these intact. CAP_CHOWN and global atomic `/etc` parent writes remain PENDING and untouched. Add no capability, broad writable directory, management wildcard or new public API. Existing host-policy renderer contract says it owns only inet vrx; therefore implement a distinct packaging-owned adapter rather than changing its table-name guard.

## Evidence

Personally inspected foundation7f35978a: bootstrap renderer/scripts; LCP Create/Update/Delete/Retrieve, registerP12/tapGatedPairs, lcpmap.HostName/FromDesired, agent applyLocked/resyncLocked, scheduler.ApplyWith and nftables runner/ALLOWLIST. Source inspection only; no implementation tests or live host commands were run for this design. Manager should assign an agent developer and independent R1/R2/R4/R5/R8 review after the narrow contract/projection choices are written down.


## Lifecycle correction: use per-membership descriptors, no generic planner change

The original singleton dependency proposal is unsafe and withdrawn. Verified `scheduler/reconciler.go:543–586`: planning substitutes NEW desired values/dependencies into nodes, then emits **all deletes before updates**. Old singleton `{A,B}` changing to `{B}` no longer depends on A; the plan deletes pairA before the singleton update revokes A. Therefore dependencies alone cannot make a singleton replacement safe.

Minimal correct shape: `base-policy.punt-interface/<hostIfName>` per dynamic admitted interface. Typed internal value `{host_if_name, lcp_pair_key}`; each membership depends on its corresponding `lcp.itf-pair/<vppName>`. Do not add generic transaction phases or planner changes for this feature. Fixed static bootstrap entries remain permanently separate and are not modeled as deletable dynamic entries; overlapping dynamic/static identity must be explicitly rejected or represented as permanent admission, never falsely promised revocable.

Personally verified traces from planner/executor code (source traces, not executed tests):

- Shrink A+B to B: nodes contain old deleted admissionA with old dependency pairA, and deleted pairA. Reverse topo DELETE admissionA precedes DELETE pairA; B unchanged.
- Grow B to B+C: forward topo CREATE pairC precedes CREATE admissionC. A failing admission create leaves nft unchanged; scheduler reverses the successful pair creation.
- Mixed remove A/add C: DELETE admissionA, DELETE pairA, CREATE pairC, CREATE admissionC. Journal reverse compensation deletes admissionC before pairC, recreates pairA before admissionA. Preserve typed partial-create/error semantics if a timeout has unknown nft outcome.
- Rename host A to D on same pair key: old admissionA is a deletion, emitted before pair update; updated pair is recreated, then new admissionD is created.
- Recreate same-name pair/type change: `executor.around` lines1112–1142 derives dependents from OLD `x.live` values, calls dependent Delete before pair recreation and Create after it. Its journal includes each dependent mutation. This is the existing appropriate typed lifecycle hook; no best-effort callback.

Each membership mutation sends one atomic nft transaction with a precise add/delete element; do not flush the complete set in each member operation. Query membership to make idempotent operations explicit; absent element deletion succeeds without exposing an arbitrary user command. Prevalidate final static+dynamic distinct union <=64 through an existing scheduler Validator/afterView; validate runtime additions against bounded actual state as well so unforeseen drift cannot overflow the cap. This ensures mixed growth/shrink can free entries before growth, without validating an impossible interim union.

Retrieve the set once per descriptor-family dump and match each non-static element to owned root-netns actual LCP pairs. Membership key is host name so stale orphan elements still get a deterministic deletable key when VPP pair disappeared; their typed readback value may carry empty pair identity and no dependency because that pair is already gone. Desired/Create values require a real pair key. Do not simply omit orphan members from Retrieve: doing so loses restart/drift cleanup. The dedicated packaging-owned dynamic set is exclusively owned by this product adapter, but static bootstrap entries must be excluded from deletion. Unknown/malformed or ambiguous actual mapping is an error, not adoption of foreign LCP pair. Consider splitting explicit static and dynamic sets/rules in the packaging base table to avoid ambiguity; that is a narrow P10 internal firewall layout decision with tests, not public schema/security privilege expansion.

Tests required before developer approval: assert exact ordered call logs for all five traces, failure at every mutation and reverse compensation, removed pair with retained other membership, orphan readback cleanup after VPP crash, same-name device reuse, static entries preserved, namespace/default-netns transitions, and max64 mixed turnover. Existing fake scheduler machinery can execute these tests without real nft/VPP privileges. This document's traces are verified against source but not a substitute for those executable tests.
