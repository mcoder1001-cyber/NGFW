# Boot-time NIC registration port — task envelope

Branch: `claude/boot-nic-registration-port` (base: `origin/main` `99a774734`)
Worktree: `/root/NGFW/.claude/worktrees/priceless-lewin-09c0d6`
Session: started 2026-10-17 by the owner's manager session from the 2026-10-17 unmerged-code audit
(`docs/status/2026-10-17-unmerged-audit-product-review.md` §1, branch `claude/review-unmerged-code-product-0351a9`).

Task: port the behaviour of the unpublished dev-host commit `76484d79f` ("fix(interfaces): register missing data NICs
automatically at boot", 2026-10-10, 15 files) onto current `main` with the `ngfw` naming; open a PR; independent review;
no merge without the owner's go-ahead.

Owned files: `apps/api/src/features/default-vpp-nics/{seed.service.ts,seed.service.test.ts,index.ts}`,
`apps/api/src/commit/commit.service.test.ts`, `apps/api/src/config.ts`, `apps/api/src/main.ts`,
`apps/api/test/e2e/default-vpp-nics.e2e.test.ts`, `deploy/systemd/ngfw-api.service`,
`deploy/debian/ngfw/tests/test_packaging.py`, `docs/decisions/PENDING-automatic-network-bootstrap.md`,
`docs/decisions/LOG.md` (one row), `docs/install/bare-metal.md`, `docs/install/upgrade.md`,
`docs/user/interfaces/default-dataplane-nics.md`, `docs/user/interfaces/basics.md`,
`apps/api/src/datastore/physical-nics.ts` (comments and the 403 wording only), `docs/status/DEFERRED-ACCEPTANCE.md`
(one row), `docs/tech-debt.md` (follow-up rows), and this task's status and review files.

Out of scope: the agent-side virtio/PCI resolution (`hostfacts.go` `netdevPCI`, `TestHostNICsVirtioPCIManagementAndData`)
is already on `main`; no lab or appliance mutation; no VPP binding or startup apply.
