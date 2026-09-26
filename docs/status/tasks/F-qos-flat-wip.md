# F-qos-flat — WIP (slot 1, started 2026-09-25 10:30)

- [x] contract(proto): QosPolicerState / QosPolicerReset + proto.md §11
- [x] contract(schema): services.qos-flat-store-source; proto fixture qos-flat-full.json
- [x] agent: DF-7 gaps (TD-11b declarations, claim-first, States/ResetIndex, qos.meta, attachment re-point),
      desired/qos.go projection + assembler, subsystems/qos.go wiring, projection hunks, rpc_qos.go, coretest model
- [x] API: state + reset routes, fake agent, e2e (7), api-client + CLI table regenerated
- [x] web: Services → QoS tab (policers, rate limits, map grid, attachments), en + fa
- [x] docs: user guide, descriptor docs, vpp-code-track V-new, questions
- [x] test/topology/qos-flat host test written (compiles, skips without VRX_INTEGRATION)
- [ ] host run + vppctl evidence + screenshots — waiting for TD-25 (host runs closed)
- [x] CI gate: PASSED on 28ea8d20 (final re-run on the last commit, see F-qos-flat.md)
