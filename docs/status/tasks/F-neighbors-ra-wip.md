# F-neighbors-ra — WIP (kept current; newest first)

- 18:19 merged task/W-seed@df67a8e (manager safety update: TD-5 rings/quiesce + D-113 rig fix) → 5a6fe26. Agent: actions/neighbors-ra (lister, flush, events), desired/neighbors_ra.go (builder+assembler), subsystems/neighbors_ra.go (registration, id range, watcher), rpc_neighbors_ra.go, Action case, coretest model + New() hook. go build/vet + internal/agent tests green. Next: unit tests, API.
- 18:05 contract commits done (schema 19935ee, proto 61412e5); proto fixture neighbors-ra-full.json; questions Q1–Q6 written.
  Next: agent (desired builder/assembler, subsystems registration + events, ListNeighbors RPC, arp flush action, coretest fake).
- 17:27 started (slot 9, base task/W-seed@8b7558e, speculative).
