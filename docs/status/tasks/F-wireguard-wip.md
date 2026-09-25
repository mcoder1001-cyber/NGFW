# F-wireguard — WIP

- [x] contract: schema `routeAllowedIps` + semantic/wireguard.ts (3 rules, tests); proto field 12, EventKind 13, rpc WireguardState + messages; regen; proto.md §11; fake stub
- [x] merged main (TD-8 seams) — 41e42258
- [x] descriptors/wireguard gap: `wireguard.meta` (agent-local names/refs table, file store), `DumpState` (state RPC helper), TD-11b declarations, PSK doc fix
- [ ] peer.go PartialCreate (waits for TD-11b on main)
- [ ] subsystems/wireguard*.go: registration, secret store (+ test-build fixture), event watcher → Env.Publish, state observer
- [ ] desired/wireguard.go builder + assembler; projection.go / subsystems.go hooks
- [ ] rpc_wireguard.go (WireguardState)
- [ ] host integration test (slot 6, ports 20610/20611)
- [ ] API feature (state, keypair, events topic), regen client, CLI table
- [ ] UI tab, en/fa, screenshot
- [ ] docs/user/vpn/wireguard.md, descriptor doc, vpp-code-track V-new
- [ ] status file, questions, CI
