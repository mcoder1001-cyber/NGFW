# P10 dynamic punt synchronization

Branch: task/P10-punt-sync-20261002; isolated worktree NGFW-packaging-punt.
Base: frozen foundation 7f35978a. Owned source: narrow agent base-policy
renderer, internal projection/registration/descriptor and their tests.
No PR61 mutation, capability changes, broad /etc writes or public schema.

## Choices and interfaces

Explicit appliance activation will load bounded root-owned non-symlink
base-policy.env, preserve permanent admin punt entries, and union only owned
root-namespace desired LCP interfaces with maximum 64 and management excluded.
The distinct renderer owns only inet ngfw_base / punt_interfaces. Commands use
fixed nft executable/argv, atomic typed stdin and bounded strict JSON readback.
No shell, wildcard, foreign table mutation, or namespace inference from an
empty pair field alone. Named/default namespaces must be accounted for.

The scheduler descriptor is an internal typed singleton; no external API.
Create dependencies alone are NOT sufficient for safe removal. Actual
reconciler.go plan constructs nodes from new desired values and deleted old
objects, and emits all deletes before desired updates. Removing pair A while
retaining singleton B deletes A before updating membership, because the new
singleton dependencies omit A. Existing admission A must be revoked before
pair deletion/recreation, with compensating state journal and failed rollback
reported DEGRADED. Manager notified; generic scheduler change needs coordinated
ownership/independent review before transaction integration.

## Tests and status

Design read in full; bounded scheduler source audit performed. No implementation
or tests yet. Next: pure renderer with validated names/union, fixed atomic nft
transaction and bounded JSON readback tests, then transaction lifecycle seam.
Scheduler integration must test delete/name change/recreate, failures and
compensation, restart/resync, disabled agents, ownership and namespace handling.
Real nft/VPP traffic and reboot NOT RUN in centralized deferred acceptance.

## Renderer/projection milestone

Pure renderer checkpoint 55917220 adds fixed command/atomic replacement and
strict bounded ifname JSON readback, with three passing Go tests. No runtime
registration, descriptor or live host calls exist yet. Corrected design cef45
withdraws singleton integration: use per-host keyed admissions carrying pair
identity, old dependencies retained for reverse deletion, and existing scheduler
recreate-dependent journal. Generic engine remains unchanged.

Pure typed admission projection now resolves explicit/default namespaces from
an explicitly known caller-provided namespace state; unknown state fails closed.
Only root pairs admitted, root host identity ambiguity rejected, permanent
admission overlap rejected, management excluded and combined limit enforced.
Four Go tests PASS including effective default-ns and foreign same-name cases.
Next command: source ../toolchain/env.sh; cd apps/agent; go test
./internal/renderers/basepolicy. Next code: precise per-element mutation and
per-host descriptor with scheduler call-order/failure tests; product activation
must supply actual default namespace and trusted configuration before use.

## Per-host descriptor milestone

Descriptor code now carries host-keyed pair dependency, validates owned actual
root pair before Create, rejects permanent overlap, and retains orphan kernel
members in Retrieve for deletion. Mutations use exact element operations with
uncertain outcome compensation; unknown Create uses PartialCreate. An existing
scheduler validator enforces final-view static/dynamic union limit.
Eleven basepolicy Go tests PASS, including executable mixed shrink/grow,
host rename, same-name type recreation with revoke-before-pair-delete order,
and orphan cleanup. Registration/product projection remains UNBUILT. Further
failure/rollback, boot and namespace integration tests are still required.
Narrow scheduler uncertainty contract independently reviewed; preserved report
P10-scheduler-uncertainty-review.md. No target host nft/VPP tests performed.

## First product integration checkpoint

Explicit NGFW_BASE_POLICY=1 requires ngfw globals owner, loads protected bootstrap
inputs and registers the per-host descriptor. Product unit enables this flag and
requires/starts after nftables. Apply, dry-run and drift/resync projections now
use the same augmentation hook; disabled agents make no namespace/nft calls.
Actual owned LCP dump feeds descriptor readback. Effective default namespace
comes from direct VPP LcpDefaultNsGet; malformed namespace readback fails closed,
without the generic descriptor's invalid-bytes-as-unset workaround.

Sixteen basepolicy tests pass, including actual adapter+scheduler ambiguous
Delete (DEGRADED, pair retained), unknown Create (PartialCreate, exact owned
cleanup before pair rollback), pair-delete failure restoring journaled admission,
mixed reverse compensation and deleted membership resync restoration. Two
subsystems tests pass for disabled/product-only activation and namespace failures.
Agent/subsystems packages compile. Runtime nft/VPP and reboot remain NOT RUN;
full hosted integration gate and fresh independent source review still needed.
Original helper and descriptor BLOCK reports preserved alongside approvals.

## Distinct dynamic ownership checkpoint

Bootstrap now creates permanent punt_interfaces plus EMPTY
 dynamic_punt_interfaces, with separate accept rules. Agent fixed commands and
strict readback target dynamic_punt_interfaces only; no permanent set mutation.
Static/dynamic overlap fails closed, final and runtime caps count permanent
inputs plus dynamic membership, and management exclusion is unchanged.
Seventeen basepolicy tests PASS; packaging suite rerun: 26 tests, 25 PASS and
1 genuine signing SKIP (5.198s). New layout requires independent review; prior
helper approvals do not cover this table-layout delta.

Product presently exposes no default-netns document projection/registration.
Defensive augmentation nevertheless prefers an explicit desired default-netns
KV over current readback, and membership depends optionally on that singleton.
Final root/nonroot precedence regression added. Actual owned pair dump supplies
effective Netns for Create proof; unknown default namespace remains rejected.

## Review and validation freeze

Latest product source 00cb2cd3 published remote 77b32a; metadata checkpoint
88cc40ad includes approved D174, installation/deferred campaign update and the
necessary reachability table entry (exact rerun PASS). Lifecycle R2 approves
00cb; first BLOCK and R2 reports preserved. Actual full local test failures are
individually classified in P10-local-go-validation.md, not relabeled as passes.
Final activation review and hosted unchanged full integration gate remain.
No target NFT/VPP operations were performed. Next command: source
../toolchain/env.sh; cd apps/agent; go test ./internal/renderers/basepolicy
./internal/subsystems -run 'TestBasePolicy|TestReachabilityTable' -count=1.
