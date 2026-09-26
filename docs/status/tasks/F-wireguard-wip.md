# F-wireguard — WIP

- [x] contract: schema `routeAllowedIps` + semantic/wireguard.ts (3 rules); proto field 12, EventKind 13, rpc WireguardState; regen; proto.md §11; fake stub
- [x] merged main twice (TD-8 seams; TD-7, TD-11b, TD-4)
- [x] descriptors/wireguard gap: `wireguard.meta`, `DumpState`, TD-11b declarations, unavailable-secret marker, PartialCreate (addendum)
- [x] subsystems/wireguard*.go (registration, secret store, test fixture loader, peer-event watcher → Env.Publish, observer)
- [x] desired/wireguard.go builder + assembler; projection.go / subsystems.go hooks
- [x] rpc_wireguard.go (WireguardState)
- [x] host checks: TestWireguardOnHost, TestWireguardHandshakeOnHost (kernel peer over a tap)
- [x] API feature (state, keypair, events topic, fake), regen client, CLI table, SDK
- [x] UI tab, en/fa, screenshots (full stack, run 2)
- [x] docs/user/vpn/wireguard.md, descriptor doc, vpp-code-track V-new
- [x] status file, questions
- [x] CI green on the D-112 squash simulation (df8c9235 = tree of dcdbd40c); the plain run fails on a gitleaks false positive in intermediate commit efcf783a (Q12)
- [ ] optional: full-stack rerun with the real-agent rollback step — waits for TD-25 (manager: no host runs that create interfaces)
