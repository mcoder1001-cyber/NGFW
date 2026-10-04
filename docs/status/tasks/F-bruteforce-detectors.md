# F-bruteforce-detectors — source completion and deferred packet acceptance

Base main06e4368c; product checkpoint5c88cb5f2b09f59a161dcfa6d72cf89b831c207a.
Contract aff92170 published equivalent remote4cb740d3ad04959045141fbd43dea18743ddd853.
Product published equivalent remote93508f8a2c7a9332d10179ca2052126ff5a069a4, PR143,
verified tree d6d31bd3de4b4ef22ddd7270f4266d9015e73655.
Manager owns publication of final report and board integration.

## What was already integrated

Main4ead8bd2 already added EventKind25/source_ip/detector, trusted root SSH journal
records, correlated charon parser, native owned secret-safe VPN watcher, lifecycle wiring,
API subscriber, Global Blocking/nftables enforcement and web-login/expiry topology driver.
The ready board row was stale; none of that existing implementation is claimed as new.

## New behavior

The previous scan path emitted only a port's first observation. A probe to port22 at0s,
a repeat at9s, and port443 at11s with a10s window missed a real distinct-port threshold2:
the API expired the first port even though the host had just observed another probe.
The host now forwards every cursor-deduplicated trusted kernel probe with canonical
`destination_port`; the API refreshes each distinct port's own timestamp, so repeats
count once and remain present while recent. Legacy missing/invalid port evidence is
ignored. Existing SSH/VPN evidence and source validation/allowlists remain in force.

Scan windows cap4096ports/source,10000sources and100000aggregate timestamps, regardless
of configured blocked-entry cap. Threshold>4096 disables scan counting with a reload
warning. Overflow discards evidence rather than fabricating a block. Expiration and
explicit clear reclaim capacity; global reclamation runs at most once/sec under capacity
pressure. Ordinary probes scan only their own bounded source window.

Topology driver now supports bound-source SSH public-key rejection with pinned host keys,
manual unblock and expiry modes. Requires actual401(web) or permission-denied(SSH)
authentication rejection; host-key/routing/rate-limit failures are not acceptance proof.
The SSH fixture must log Failed publickey records (authorized lab LogLevel VERBOSE).

## Actual verification

```
pnpm --filter @ngfw/api exec vitest run src/features/auto-block/engine.test.ts src/features/auto-block/publisher.test.ts src/features/auto-block/port-window.test.ts src/features/auto-block/detector-events.test.ts
Test Files 4 passed (4)
Tests 31 passed (31)
```

API typecheck passed after proto/schema/yang dependency builds. Targeted API ESLint
passed (existing root package module-type warning only). Scan tests cover repeat refresh,
exact window boundary, source isolation, unreachable threshold, per-source/source/aggregate
budgets and reclamation. Service tests exercise actual Bus subscription → observe path,
missing/malformed ports, lifecycle unsubscribe and allowlisted/mapped/loopback sources.

```
GOTOOLCHAIN=local GOMAXPROCS=2 GOFLAGS=-p=2 go test -race -count=1 ./internal/detectors ./internal/agent -run 'Test(TrustedSSH|Charon|ScanPort|Native|AutoBlock)'
ok ngfw/agent/internal/detectors 1.097s
ok ngfw/agent/internal/agent 1.284s
```

Go vet of the same packages passed. New event regression checks destination_port,
source and detector survive the filtered event subscriber with timestamp/sequence.

```
python3 -m unittest discover -s test/topology/autoblock -p 'test_*.py' -v
Ran 4 tests
OK
```

Driver syntax compiles. These Python simulated results verify the acceptance driver,
not real network enforcement. Source gate with pinned gitleaks passed. Full quick gate BLOCKED-ENV at /tmp/ngfw-ci/detectors-20261004-045756-2.
Generator output clean; source/security gates passed. During concurrent full-project
Turbo, API tests were killed and API typecheck exited137; this is resource/environment
failure, not a source assertion result. Manager instructed no repeat after independently
observed baseline environment failures; own gate interrupted through session67971
(exit130) after its failure log. No CI GATE PASSED claim. Focused31tests and full API
typecheck ran successfully before the heavy concurrent gate.

## Remaining acceptance and limits

Live VPP/nftables local-in/forwarding block, removal/expiry and anti-lockout packet
acceptance are NOTRUN in the cloud workspace; run the topology driver under the real
lab's slot/shared lock and attach output. No host services, journal or VPP process changed.

Native VPN requires established secret-safe state capability and can miss short-lived
failed SAs between polls. Charon remains compatibility/test-peer parsing and never
authorizes a product native VPN block. Deploy matching agent/API together because
legacy port-less scan events cannot supply trustworthy distinct-port evidence.
At memory/log/event-stream saturation scan detection can miss attacks. Existing web/SSH
failure timestamp windows have their inherited retention behavior; the new fixed scan
budgets do not redesign that independent preexisting engine.
