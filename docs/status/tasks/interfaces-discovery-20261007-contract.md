# Additive read-only REST contract

`GET /api/v1/state/interfaces` retains its prior running/retrieved/live/counters semantics and adds optional `hostInventory`, `inventoryOnly`, `hostInventoryStatus`, `dataplaneStatus`, and source-specific `observationErrors`.
The existing agent `HostNics` RPC supplies only physical PCI NICs; no protobuf/schema shape change is necessary. The generated API client was regenerated using `pnpm gen`, never hand edited. D-240 chooses observation over mutation: GET creates no candidate/running configuration and changes no NIC ownership. Configured PCI identity is authoritative; non-authoritative MAC matching requires uniqueness and a physical engine type.

Contract checkpoints: 859721ec8 and c450739df14ee4abd97f021b71d5a8104222fa00 (second finishes generator formatting; final net generated change is 21 additive lines). Manager reviews contract and preserves history before final D112 squash.
