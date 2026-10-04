# F-pim-frrsync independent T1/T2/T3 verification

Tested source HEAD: `62535f7e18be3e41fcf0784aeecda7ef932bd785`. Date: 2026-10-04 UTC. No live slot assigned; no host mutations.

Focused T1 unit/race/vet and source/security checks PASS. Complete mandatory quick gate verdict: **BLOCKED-ENV**, awaiting unchanged complete hosted quick CI; manager previously reproduced baseline Unix socket EPERM and chown EINVAL failures three times and instructed no further full local rerun. Scoped PASS is not aggregate CI PASS.

T2: **BLOCKED-ENV** — no PostgreSQL/Valkey executable or real slot stack available; API Vitest/fakes are unit evidence only.
T3: **BLOCKED-ENV** — no VPP/vtysh executable or /run/vpp/api.sock available; no real FRR/VPP forwarding, restart, removal/expiry or topology acceptance performed. No waiting on shared lab locks (unavailable environment confirmed immediately). Harness and documentation do not claim fake/Python acceptance proves network enforcement.

Generated directory status is clean and unchanged against main. Actual gen-check output below completed successfully. PIM/LDP guards and race/vet reran after R8 product fixes; the successful prior generation remains applicable because contracts/generated directories unchanged.

| Scope | Observed | Result |
|---|---|---|
| Focused race/vet | Actual output below; vet silent EXIT0 | PASS |
| Source/security/contract/slot guards | Pinned gitleaks, check PASSED | PASS |
| Generator | gen-check PASSED, unchanged tracked output | PASS |
| Complete quick CI | Known baseline host permission failure, hosted pending | BLOCKED-ENV |
| Real PostgreSQL/Valkey | Unavailable | BLOCKED-ENV |
| Real VPP/FRR topology | Unavailable | BLOCKED-ENV |

## Actual output: pim-final.log

```text
ok  	ngfw/agent/internal/frrsync/pim	1.145s
ok  	ngfw/agent/internal/descriptors/mfib	1.034s
ok  	ngfw/agent/internal/renderers/frr/pim	1.077s
ok  	ngfw/agent/internal/subsystems	1.084s

== contract guard: HEAD vs main ==
no contract files changed in the 8 commit(s) of HEAD since main (06e4368c)

== forbidden patterns (+ gitleaks) ==
WARN control-plane lines exempted with 'ALLOW:' — reviewer, check each justification:
      apps/api/src/features/mgmt-tls/mgmt-tls.test.ts:5:import { execFileSync } from 'node:child_process'; // ALLOW: test-only openssl cert
      apps/api/src/features/mgmt-tls/mgmt-tls.test.ts:19:    execFileSync('openssl', ['version'], { stdio: 'ignore' }); // ALLOW: test-only openssl cert
      apps/api/src/features/mgmt-tls/mgmt-tls.test.ts:26:const openssl = (args: string[]) => execFileSync('openssl', args, { stdio: 'ignore' }); // ALLOW: test cert
      apps/api/src/features/notifications/sinks.test.ts:1:import { execFileSync } from 'node:child_process'; // ALLOW: fixed-argument openssl creates temporary test-only TLS certificate; no runtime exec, shell or committed key
      apps/api/src/features/notifications/sinks.test.ts:103:  execFileSync( // ALLOW: fixed-argv test-only OpenSSL generates a temporary SMTP TLS certificate; no runtime shell
      apps/api/src/features/pki/testkit.test.ts:3:import { execFileSync, spawnSync } from 'node:child_process'; // ALLOW: test-only verifier (openssl), never product code
      apps/api/src/features/pki/testkit.test.ts:32:  const result = spawnSync('openssl', args, options); // ALLOW: test-only fixed-argv verifier
      apps/api/src/features/pki/testkit.test.ts:44:  return execFileSync('openssl', args, { input, stdio: ['pipe', 'pipe', 'pipe'] }); // ALLOW: test-only verifier
ok: no shell/VPP/FFI access in apps/api/src apps/web/src packages/*/src
ok: no Dockerfile/compose files
ok: no kill-by-pattern in scripts
ok: no secret-shaped strings
ok: ngfwtestsecrets only in test code
ok: gitleaks — scanned ~51641 bytes (51.64 KB) in 192ms no leaks found

== packet-trace ban on the shared VPP (D-128) ==
ok: no packet trace (trace add / show trace / clear trace / tracedump API) outside docs and the generated bindings

== slot resource scheme (1..32, no collisions) ==
ok: 30 developer slots + CI slot 12; 964 ports, 32 id ranges, no collision; tools/lab env verified for 31 slots
check PASSED (0m02s)

```

## Actual output: pim-guards.log

```text

== contract guard: HEAD vs main ==
no contract files changed in the 7 commit(s) of HEAD since main (06e4368c)

== forbidden patterns (+ gitleaks) ==
WARN control-plane lines exempted with 'ALLOW:' — reviewer, check each justification:
      apps/api/src/features/mgmt-tls/mgmt-tls.test.ts:5:import { execFileSync } from 'node:child_process'; // ALLOW: test-only openssl cert
      apps/api/src/features/mgmt-tls/mgmt-tls.test.ts:19:    execFileSync('openssl', ['version'], { stdio: 'ignore' }); // ALLOW: test-only openssl cert
      apps/api/src/features/mgmt-tls/mgmt-tls.test.ts:26:const openssl = (args: string[]) => execFileSync('openssl', args, { stdio: 'ignore' }); // ALLOW: test cert
      apps/api/src/features/notifications/sinks.test.ts:1:import { execFileSync } from 'node:child_process'; // ALLOW: fixed-argument openssl creates temporary test-only TLS certificate; no runtime exec, shell or committed key
      apps/api/src/features/notifications/sinks.test.ts:103:  execFileSync( // ALLOW: fixed-argv test-only OpenSSL generates a temporary SMTP TLS certificate; no runtime shell
      apps/api/src/features/pki/testkit.test.ts:3:import { execFileSync, spawnSync } from 'node:child_process'; // ALLOW: test-only verifier (openssl), never product code
      apps/api/src/features/pki/testkit.test.ts:32:  const result = spawnSync('openssl', args, options); // ALLOW: test-only fixed-argv verifier
      apps/api/src/features/pki/testkit.test.ts:44:  return execFileSync('openssl', args, { input, stdio: ['pipe', 'pipe', 'pipe'] }); // ALLOW: test-only verifier
ok: no shell/VPP/FFI access in apps/api/src apps/web/src packages/*/src
ok: no Dockerfile/compose files
ok: no kill-by-pattern in scripts
ok: no secret-shaped strings
ok: ngfwtestsecrets only in test code
ok: gitleaks — scanned ~45068 bytes (45.07 KB) in 330ms no leaks found

== packet-trace ban on the shared VPP (D-128) ==
ok: no packet trace (trace add / show trace / clear trace / tracedump API) outside docs and the generated bindings

== slot resource scheme (1..32, no collisions) ==
ok: 30 developer slots + CI slot 12; 964 ports, 32 id ranges, no collision; tools/lab env verified for 31 slots
check PASSED (0m06s)

== generate + generated-output gate ==
clean: packages/proto/gen apps/agent/gen packages/schema/dist packages/api-client/src/generated
gen-check PASSED (0m42s)

```

Commands (PATH includes shared go/bin and ci-tools; GOTOOLCHAIN=local GOMAXPROCS=2 GOFLAGS=-p=2):

```text
tools/ci.sh check --base main
tools/ci.sh gen-check
cd apps/agent
go test -race -count=1 ./internal/frrsync/pim ./internal/descriptors/mfib ./internal/renderers/frr/pim
go test -race -count=1 ./internal/subsystems -run TestPim
go vet ./internal/frrsync/pim ./internal/descriptors/mfib ./internal/renderers/frr/pim ./internal/subsystems ./internal/agent
```
