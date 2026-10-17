# Boot-time NIC registration port — checkpoint

Branch: `claude/boot-nic-registration-port`; base `origin/main` `99a774734`.
Remote: published to `origin/claude/boot-nic-registration-port` (SHA recorded in the PR; see git log).
Owned files: see `boot-nic-registration-port-envelope.md`.

## Completed code

- `apps/api/src/features/default-vpp-nics/seed.service.ts`: `SeedService` registers the non-management host NICs
  missing from the running document at every boot through one system commit. `seedDocument` matches NICs by PCI
  (case-insensitive), keeps `dataplane.devices.<pci>.name`, device tuning, the whitelist, existing rows and released
  (`owner: host`) NICs, adopts an existing row of the same name (settings kept, marker added), skips bonds, second
  physical rows, reserved (`local0`, `loopN`, `host-*`) and invalid names, and appends discovered management PCIs only
  when something was registered. Saved `dataplane.managementPci` flags NICs as management before the no-management
  check. `no-nics` is retried; `OnModuleDestroy` stops the watch and timer; `start()` is idempotent.
- `deploy/systemd/ngfw-api.service`: comment on the kept `NGFW_SEED_DEFAULT_NICS=1`;
  `deploy/debian/ngfw/tests/test_packaging.py`: `test_api_unit_registers_default_nics_before_site_overrides`.
- `apps/api/src/config.ts`, `main.ts`, `features/default-vpp-nics/index.ts`: comments.
- Tests: `seed.service.test.ts` (19 cases), `commit.service.test.ts` (+2 cases through real gRPC / fake agent),
  e2e wording.
- Docs: `docs/decisions/PENDING-automatic-network-bootstrap.md` (implemented state, owner instruction 2026-10-10),
  `docs/decisions/LOG.md` D-249, `docs/user/interfaces/default-dataplane-nics.md`, `docs/user/interfaces/basics.md`,
  `docs/install/bare-metal.md`, `docs/status/DEFERRED-ACCEPTANCE.md` (appliance reboot case).

Not ported (already on `main`): `apps/agent/.../hostfacts.go` `netdevPCI` virtio handling and
`TestHostNICsVirtioPCIManagementAndData`; `seed.golden.json` (main's copy already carries the newer tunnel domains).

## Actual test results

```
$ python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_packaging.py -v
test_api_unit_registers_default_nics_before_site_overrides ... ok
Ran 17 tests in 1.263s
OK

$ pnpm turbo run lint typecheck test --filter=@ngfw/api
@ngfw/api:test:  ✓ src/commit/commit.service.test.ts (26 tests) 6454ms
@ngfw/api:test:  ✓ src/features/default-vpp-nics/seed.service.test.ts (19 tests) 528ms
@ngfw/api:test:  Test Files  113 passed (113)
@ngfw/api:test:       Tests  806 passed (806)
 Tasks:    10 successful, 10 total
Cached:    3 cached, 10 total
  Time:    2m38.844s
```

`tools/ci.sh quick`: running (result appended below when finished).

## Remaining

Complete `tools/ci.sh quick`, open the PR against `main`, independent review (R1, R2, R7, R8; R3 not needed — no
contract change). No merge without the owner's go-ahead.

Current failure: none observed.
Exact next command: `tools/ci.sh quick` (then `gh pr create`).
