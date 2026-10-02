# P10 dynamic LCP punt-set implementation handoff

Design branch `task/P10-punt-design-current`, exact foundation `7f35978a`. Read-only investigation; no product/host mutations. P10 requires working punt paths in the packaging-owned base firewall; bootstrap CSV alone is not dynamic synchronization. This is implementation work, not an acceptance test deferred for missing lab access.

## Existing seams and failure

`deploy/debian/vrx/assets/render-base-policy.py:render` builds only `inet vrx_base`, an explicit `type ifname` set `punt_interfaces`, and separate management 22/443 rules; max64, unique safe names, management overlap rejected. `firewall-bootstrap.sh` saves nonsecret authoritative interface inputs in `/etc/vrx/base-policy.env`. No production agent code updates this set afterward. A new LCP pair is therefore dropped by the priority -10 base chain even if the independently owned `inet vrx` host-policy accepts it.

`subsystems/frr.go:registerP12` registers `tapGatedPairs`; its `Retrieve` delegates to owned `lcp.ItfPairDescriptor.Retrieve`. `lcp/lcp.go:Create` creates the Linux pair and persists ownership; `Delete` re-resolves ownership and removes it safely; Update returns ErrRecreate. `lcpmap.HostName` / FromDesired supply validated desired host names. `agent/service.go:applyLocked` and `resyncLocked` run the scheduler under existing transaction serialization. `scheduler.ApplyWith` owns rollback/verification. Use these seams rather than an eventually consistent post-commit watcher that could report APPLIED before firewall failure.

## Recommended minimal typed implementation

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
