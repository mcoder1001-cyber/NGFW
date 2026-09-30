# network-defaults-web — UI changes and boot prerequisites

Status: **partial implementation; boot-policy answers pending; not deployment-ready**.

On 2026-09-30 the product owner explicitly requested merging all completed changes.
That authorizes merging this implementation; management-NIC selection and migration
policy remain unanswered and automatic boot/TAP activation remains follow-up work.

## Request and scope

The product owner requested neutral web terminology instead of FRR/VPP/strongSwan,
removal of manual Linux routing-interface controls, automatic data-NIC ownership at
boot except management, and automatic routing TAP configuration.

Implemented independently of the pending boot-policy decisions:

- English/Persian product text, schema metadata and engine-choice display labels.
- Service naming in diagnostics and log/event presentation; original records and
  exports remain truthful, and user-selected identifiers are not globally rewritten.
- Schema-aware configuration diff display, preserving the actual API changes.
- Removal of BGP's manual Linux-pair tab and hidden generic `lcp` controls/routes.
  Existing custom names/types/namespaces survive adjacent saves; absent pairs stay
  absent. Regression tests inspect real PATCH bodies.
- Structured startup status in place of implementation-specific raw file previews.
- Read-only PCI inventory safety prerequisite, including real virtio child layout,
  already bound VFIO devices, management exclusions, IOMMU membership checks,
  path containment, unsupported/ambiguous-device issues and repeat-read stability.

The inventory has **no production caller** and changes no device ownership. No boot
unit, NIC rebinding, initial configuration migration or automatic TAP activation has
been added. In particular, current routing validation still requires existing explicit
pairing: these UI changes must ship together with the pending backend work.

## Decisions needed

See [PENDING-automatic-network-bootstrap](../../decisions/PENDING-automatic-network-bootstrap.md).
The two questions sent in chat concern persistent management-NIC selection and the
migration behavior of existing installations. No answer was inferred from elapsed time
or a preselected option.

Follow-up implementation must persist discovered interface configuration through the
datastore, configure required plugins, establish correct service ordering, preserve
existing mappings, and prove restart/rollback behavior. Do not introduce ordinary
routing-descriptor dependencies on each TAP: pair churn would reset unrelated sessions.
Existing management discovery also needs virtio device-ancestry handling before it can
infer that hardware without an explicit management PCI address.

## Validation environment

Node 22.23.3, pnpm 12.5.1, Go 1.26.8 and Buf 1.73.0. Go/Buf release checksums were
verified; protobuf generators match the committed headers. Local tool/cache paths are
outside the checkout. `TURBO_ENV_MODE=loose` preserves the writable cache paths for
generation in this cloud machine; `umask 022` avoids the cloud's default `0077` changing
the assumptions of an existing file-permission restoration test.

Completed checks:

```text
pnpm gen:check
clean: packages/proto/gen apps/agent/gen packages/schema/dist packages/api-client/src/generated
gen-check PASSED (1m24s)

go vet ./...                         # apps/agent: passed
umask 022; go test -count=1 ./...     # apps/agent: passed
go build ./...                       # apps/agent: passed
122 packages with passing tests; 162 packages with no test files

go test ./internal/renderers/vppstartup ./cmd/vrx-startupgen
go vet ./internal/renderers/vppstartup ./cmd/vrx-startupgen
golangci-lint run ./internal/renderers/vppstartup ./cmd/vrx-startupgen
0 issues

pnpm --filter @ngfw/ui-kit build
pnpm --filter @ngfw/ui-kit lint
pnpm --filter @ngfw/ui-kit typecheck
pnpm --filter @ngfw/ui-kit test --maxWorkers=2
Test Files 18 passed (18)
Tests 90 passed (90)
```

The first full web run found two presentation regressions and one stale module from
an edit during the run. The result-key regression was fixed without changing its existing
test assertion; dashboard event wording retains the existing localized engine label.
The final run used stable source and the rebuilt UI kit:

```text
pnpm --filter @ngfw/web lint
check-logical-css: OK (427 files, no physical left/right CSS)
pnpm --filter @ngfw/web typecheck
pnpm --filter @ngfw/web test --maxWorkers=4
Test Files 96 passed (96)
Tests 557 passed (557)

pnpm --filter @ngfw/web build
bundle-budget: initial 487.6 kB gzipped of 600.0 kB budget; 131 lazy chunk(s)
bundle-budget: OK
check-no-dev-routes: OK (133 files, no /dev routes, dev nav or demo chunks)

bash tools/ci.sh check
check PASSED (0m17s)
git diff --check                     # passed
```

All 647 web/UI-kit tests pass. Generated contracts, dependency manifests and lockfiles
remain unchanged. The complete repository `quick` gate was initially blocked by missing
generation tools; after installing them, generation, the changed-package checks and the
whole agent unit suite were run separately as recorded above. A full `quick`/`full` gate
completion is not claimed.

Live VPP, physical NIC takeover, reboot, routing-daemon integration and browser E2E
against a running appliance are **not run**. Unit-mode Go integration tests remain
opt-in/skipped. No live network configuration was changed, no commit or merge was made,
and no deployment or environment publication was performed during that validation stage.
