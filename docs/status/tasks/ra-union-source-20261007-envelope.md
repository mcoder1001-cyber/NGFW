# RA reviewed union recovery

Owner /root/ra_union_developer; branch codex/ra-union-source-20261007; isolated worktree /root/ngfw-wt/ra-union-source-20261007. Base f6ae6e555. Source reviewed union 711e18e12ac2776ad4fc15066f811b60599e63f2, predecessor b0367efb48339808359c4c82e2675060d8c1ddd4 (PR193 now merged).

Owned files: apps/agent/internal/ra_vpn/**; apps/agent/internal/agent/rpc_ra_vpn.go and rpc_ra_vpn_test.go; apps/agent/internal/renderers/strongswan/renderer.go and lazy_templates_test.go; apps/agent/go.mod and go.sum (exact reviewed godbus pin only); apps/web/src/domains/vpn/ra-vpn/RaVpnPage.tsx and RaVpnPage.test.tsx; apps/web/src/locales/{en,fa}/ra-vpn.json; deploy/debian/ngfw/prepare.sh and tests/test_prepare.py (RA helper compile flags only); test/topology/ra-vpn/actual-stop-coordinator.py; docs/status/tasks/ra-union-source-20261007*.

Only apply reviewed RA delta, preserve subsequent main changes. No board, source contract, services, shared VPP, privileges or external fixtures modified. Original proof/identity/deadline/profile guards retained. Targeted race/unit/compile checks only, no aggregate CI per owner waiver. No live acceptance claimed. TMPDIR=/rau.
