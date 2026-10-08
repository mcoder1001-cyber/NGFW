# Owner-authorized completion campaign

Owner direction, 2026-10-08: complete the non-laboratory work, merge reviewed
changes, and reconcile board/documentation. Run the final aggregate CI campaign
after source corrections are complete; no intermediate workflow was requested.
The earlier PR206 navigation-only hold is superseded by this explicit all-merge
direction. This instruction does not certify unfinished code or laboratory tests.

Integration branch: `codex/completion-20261008`, PR214. Main at campaign start:
`417e8fcd0c82fae7906da0fba1c0622a8de017f7`. Checkpoints carry `[skip ci]`.
The required quick gate is not weakened. Offline Python fixture coverage is
extended to include the new committed release lock and provenance contracts.

## Source staged for final validation

| Scope | Reviewed source provenance | Evidence and boundary |
|---|---|---|
| FRR slot cleanup | PR209 `64171651` | Three race regressions passed; prior source CI37812533974 passed. Does not resolve native mgmtd deadline or current 200-route proof. |
| PPP lifecycle | PR210 `baa12c54` | Reviewed fencing, parent lifetime and transition retries; does not implement missing carrier. |
| Web navigation | PR206 `0df43e59` | Independent source reviews and earlier full local/hosted quick passed; en/fa navigation source staged unchanged. |
| TD19 Python release | PR212 `bfe917b6` | Six direct pins,22 runtime wheels,24 upstream artifacts and reproducible derived wheel. Independent source/security approval and offline rebuild. Actual Ubuntu26.04 install/boot/packet remains unverified. |
| Dynamic Multi-WAN | PR215, product local `edd8d0d1` | R4-approved DHCP renewal/release/error withdrawal, cleanup retry protection, fixed probe executable and egress-generation fencing. Actual PPP runtime binding remains pending. |
| Production sealed credentials | `9d61005d`, remote `eebb6254` | Independent R2 combined approval and R4 CA-key isolation approval;36 API tests and eight focused race packages passed. WG/SNMP/BGP/NTP/TLS syslog/host namespace startup and projection bound to existing versioned store. |
| IP unnumbered | `675cd9ea`, integration remote `036ca641` | R4-approved generated-API descriptor, ownership/dependency guards and Apply/Retrieve/restart/rollback lifecycle race tests. Native VPP acceptance still unverified. |
| Daemon ownership/identity | PR213, product local `4b39ad1f` | R2-approved capability/mount boundary and atomic public-link migration;10 local Python cases and sysident race passed. Two real foreign-UID/capability cases require final hosted execution. No appliance service changed. |

## Previous CI failure repaired without changing production guards

Earlier PR210 workflow37812958217 on `a9398271`, started before CI deferral,
failed solely at `TestPppoeIPv6MirrorFollowsHookState`: the render-only non-owner
fixture invoked a guarded hook without supervisor admission. The real hook
correctly refused to publish. Integration commit `3d03bab6` explicitly admits
the simulated session through `ResumeIPv6` before the existing hook assertions.
Author and independent R1 each ran this test three times under race detection,
all passing, no skips (1.337s/1.340s). The current final aggregate gate has not
run. The earlier local procfs/PID namespace limitations remain recorded and are
not converted into passing process-lifetime acceptance.

## Still in progress / unavailable

- Accepted WBS D1.3 IP unnumbered and the six production credential consumers
  now have reviewed source staged above. The discovered syslog CA-key alias
  escape is corrected before private-key lookup, independently rechecked.
  NTS server certificates remain outside the accepted chrony feature scope.
- Setup wizard: the accepted feature prompt explicitly includes PPPoE after
  client completion. Its disabled option is being replaced with a tested additive
  setup contract and distinct logical WAN/physical parent configuration, using
  existing sealed credential references and preserving confirmed-commit safeguards.
- PPP kernel carrier: exclusive physical L2 transport, distinct logical VPP
  transit interface, owned namespace and pre-local return routing, lifecycle
  readiness/withdrawal, effective policy identity, dynamic WAN probing and PD
  LAN address/RA application. Allocator or helper fixtures alone do not complete it.
- Product license: owner selected use of existing text, but supplied no text or
  identifier, and no authoritative product LICENSE/copyright was found. No
  license or copyright holder is invented. This is a release input, not a lab test.
- Final unchanged quick gate and relevant fixture workflows are deferred until
  source is ready. Native RA supplier/identity, P12 mgmtd, PPP and other packet,
  installed-service, restart/rollback and browser acceptance remain unverified.

Next: finish and independently review carrier/PD/WAN integration, consolidate
source, then run final CI on the exact cumulative candidate. Preserve failures,
fix their actual causes and obtain passing required checks before integration.
Refresh this receipt and task states from actual merge and validation results.

Board reconciliation: historical merged receipts are retained, but P08 is reopened
to review. WG/SNMP/host services/BGP now have independent source approval and
are in review awaiting final integration; Multi-WAN remains running for actual
PPP runtime binding. Task counts therefore reflect incomplete work rather
than preserving an inaccurate completion percentage.

Integrated-tree focused check after credential/unnumbered registration merge:
`go test -race -count=1 ./internal/agent -run
'TestHostCredentialProjectionSelectsOwnerAndRotation|TestUnnumberedDomainApplyRetrieveRevoke'`
PASS 1.191s; no skips and no full CI run.
