# P10 dynamic punt synchronization

Branch: task/P10-punt-sync-20261002; isolated worktree NGFW-packaging-punt.
Base: frozen foundation 7f35978a. Owned source: narrow agent base-policy
renderer, internal projection/registration/descriptor and their tests.
No PR61 mutation, capability changes, broad /etc writes or public schema.

## Choices and interfaces

Explicit appliance activation will load bounded root-owned non-symlink
base-policy.env, preserve permanent admin punt entries, and union only owned
root-namespace desired LCP interfaces with maximum 64 and management excluded.
The distinct renderer owns only inet vrx_base / punt_interfaces. Commands use
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
