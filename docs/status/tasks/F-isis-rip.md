# F-isis-rip — IS-IS and RIPv2 via FRR (RIPng blocked on contract)

State: review. Branch `claude/modest-keller-upaw4m`. Built on the F-ospf template.

## Done
- Agent: `renderers/frr/isis` — `isis` section (470), `ip/ipv6 router isis ngfw` + `isis …` interface lines (S2),
  state reader `isisNeighbors`, poller `isis-adjacencies`; golden, 16 error cases (NET, level/circuit-type
  compatibility, hostile names, …), adjacency parser + poller test. `renderers/frr/rip` — `rip` section (420), golden,
  13 error cases. References: `docs/agent/renderers/frr-{isis,rip}.md`.
- Wiring: `subsystems/isis_rip.go` (blank imports); `desired/bgp.go` FRRDoc/AssembleFRR carry `routing.isis` and
  `routing.rip` (same minimal edit as F-ospf); `projection.go` isis/rip `handled: true`; `project_p12_test.go` now
  expects only `/routing/bfd` as unsupported (bfd is the last unimplemented routing leaf).
- Web: WEB-4a `IsisRipPage` routed at `/routing/isis-rip` (router + nav anchors), nav.test available list, OspfPage
  test and the page's own test assert reachability.

## Checks
- `go build ./... && go vet ./...` clean; `go test ./...` green except `internal/contracttest` TestSchemaProtoDrift*,
  identical failure set on the untouched base.
- golangci-lint on touched packages: 0 issues. Web: tsc clean, eslint clean on touched files, vitest green.

## Not tested / not done
- No FRR / netns lab in this environment: no frrtest integration, no VPP FIB evidence, no restart/rollback evidence;
  FRR line acceptance and the `show isis … json` field names are unverified against 10.7.
- Not built (need an additive contract or more work): RIPng (`routing.ripng`), `rip.version`, IS-IS per-family switch
  and passwords, EventKind 21, `lcp.osi-proto` descriptor (OSI punt), RIP state readers (no FRR JSON), API state
  endpoints, live adjacency grid in the UI, user docs, schema rule circuitType-vs-level (enforced in the renderer only).

## Open questions
- Make the RIPng / IS-IS auth / families contract a follow-up row?
- Who enables the irreversible OSI punt on a real box (globals owner when `routing.isis` is present?).
