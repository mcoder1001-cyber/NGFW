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
