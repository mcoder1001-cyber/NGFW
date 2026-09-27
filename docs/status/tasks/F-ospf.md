# F-ospf — OSPFv2 via FRR (OSPFv3 blocked on contract)

State: review. Branch `claude/modest-keller-upaw4m`.

## Done
- Agent: `apps/agent/internal/renderers/frr/ospf` — `ospf` section (order 440), `ip ospf …` interface lines via the
  framework's S2 seam, state readers `ospfNeighbors` / `ospfInterfaces`, poller `ospf-neighbors`. Golden render,
  21 error cases (hostile names, undefined area, backbone stub/NSSA, …), neighbour JSON parser + poller tests.
  Reference: `docs/agent/renderers/frr-ospf.md`.
- Wiring: `subsystems/ospf.go` (blank import); `desired/bgp.go` FRRDoc/AssembleFRR carry `routing.ospf` (small edit in
  a P12 file — without it the section never sees the document); `projection.go` ospf leaf `handled: true` under the
  F-ospf anchor; `project_p12_test.go` now uses `isis` as the unsupported example.
- Web: WEB-4a's `OspfPage` routed at `/routing/ospf` (router + nav under the F-ospf anchors), nav.test available list
  and the page's own nav test updated.
- TD-11a reachability: `frr/ospf` is a sub-package of the already-wired `frr` renderer; the table only lists top-level
  packages, so there is no entry to flip and maxPending (23) is unchanged. No new descriptor → no TD-11b ownership
  declaration, no ID allocation (TD-8b n/a).

## Checks
- `go build ./... && go vet ./...` clean; `go test ./...` all green except `internal/contracttest`
  (TestSchemaProtoDrift*), which fails identically on the untouched base (pre-existing schema/proto drift, not F-ospf).
- golangci-lint on touched packages: 0 issues.
- web: tsc, eslint (touched files), vitest 77 files / 478 tests green.

## Not tested / not done
- No live FRR (frrtest) or topology run: the netns/veth rig, VPP FIB acceptance, restart and rollback evidence could not
  be produced in this environment. FRR line order is unverified against 10.7.
- OSPFv3, MD5 auth, EventKind 20, OSPF API state endpoints, UI neighbours grid, user docs: not built (need the additive
  contract `routing.ospf6` / `OspfInterface.auth` and API work beyond this pass).

## Open questions
- Make the OSPFv3 + auth contract (RoutingConfig 13, OspfInterface 9, EventKind 20) as a follow-up row?
- Does linux-cp deliver 224.0.0.5/6 to the tap (host check)?
