# F-images: independent T1 verification

Exact frozen tested SHA: bf9fa4b25a62c4009229ab23c9d0a4480e507953. Date 2026-10-05. Slot none; unit-only complete gate. Detached isolated worktree /root/ngfw-wt/r1-gate-images-20261005. No developer tree mutation.

Command actually run:
```sh
TMPDIR=/root/.cache/review-r1-tmp GOMAXPROCS=2 GOFLAGS=-p=2 NGFW_CI_TASK_CONCURRENCY=2 tools/heavy.sh tools/ci.sh quick --base origin/main
```

Actual output (full log /root/ngfw-wt/review-r1-seven-20261005/images-quick.log):
```text
== summary (quick) ==
  contract guard: HEAD vs origin/main                0m00s
  tools (golangci-lint, gitleaks)                    0m02s
  install (pnpm --frozen-lockfile --prefer-offline)   0m15s
  generate + generated-output gate                   1m34s
  forbidden patterns (+ gitleaks)                    0m06s
  packet-trace ban on the shared VPP (D-128)         0m02s
  ip classify reset after every shell interface create (D-185)   0m01s
  slot resource scheme (1..32, no collisions)        0m03s
  lint · typecheck · unit tests · build (turbo)   1m10s
  apps/agent: make lint test build                  17m25s
  apps/cli: make lint test build                     0m31s
  test/ Go modules, unit mode (test/integration/reachability test/integration/smoke test/topology/acl test/topology/bonding test/topology/bridge-l2 test/topology/det44 test/topology/host-acl-nftables test/topology/images test/topology/interfaces test/topology/ipfix-sflow test/topology/kea-dhcp-relay test/topology/loopback-bvi-gso-lldp-span test/topology/nat44-ed-sessions test/topology/nat44-ei-64-66-nptv6 test/topology/neighbors-ra test/topology/object-model test/topology/qos-flat test/topology/system-identity test/topology/traffic-a/globals test/topology/unbound-chrony-syslog test/topology/vlan-qinq test/topology/vrf-static-ecmp)   1m39s
  deploy/vpp: shellcheck + apply-startup fake-host harness   5m07s
  warnings:
    - control-plane lines exempted with 'ALLOW:' — reviewer, check each justification:
      apps/api/src/features/mgmt-tls/mgmt-tls.test.ts:5:import { execFileSync } from 'node:child_process'; // ALLOW: test-only openssl cert
      apps/api/src/features/mgmt-tls/mgmt-tls.test.ts:19:    execFileSync('openssl', ['version'], { stdio: 'ignore' }); // ALLOW: test-only openssl cert
      apps/api/src/features/mgmt-tls/mgmt-tls.test.ts:26:const openssl = (args: string[]) => execFileSync('openssl', args, { stdio: 'ignore' }); // ALLOW: test cert
      apps/api/src/features/notifications/sinks.test.ts:1:import { execFileSync } from 'node:child_process'; // ALLOW: fixed-argument openssl creates temporary test-only TLS certificate; no runtime exec, shell or committed key
      apps/api/src/features/notifications/sinks.test.ts:103:  execFileSync( // ALLOW: fixed-argv test-only OpenSSL generates a temporary SMTP TLS certificate; no runtime shell
      apps/api/src/features/pki/testkit.test.ts:3:import { execFileSync, spawnSync } from 'node:child_process'; // ALLOW: test-only verifier (openssl), never product code
      apps/api/src/features/pki/testkit.test.ts:32:  const result = spawnSync('openssl', args, options); // ALLOW: test-only fixed-argv verifier
      apps/api/src/features/pki/testkit.test.ts:44:  return execFileSync('openssl', args, { input, stdio: ['pipe', 'pipe', 'pipe'] }); // ALLOW: test-only verifier
  mode quick · wall time 27m57s · logs /root/ngfw-wt/logs/ci/r1-gate-images-20261005-20261005-064622-1545967

CI GATE PASSED
```

| Scenario | Expected | Observed | Result |
|---|---|---|---|
| Contract/secret/slot guards | unchanged guards green | all passed | PASS |
| Normal pnpm generation | no generated changes | generated gate passed; final git status empty | PASS |
| Complete TS lint/typecheck/unit/build | all scheduled packages green | 35 successful,35 total | PASS |
| Agent and CLI lint/race/tests/build | no failed package | both steps passed | PASS |
| Image Go wrapper + Python format/safety tests | actual source tests run | ok ngfw/test/topology/images 2.511s | PASS |
| Every test Go module and fake-host startup harness | no failures | all modules; sharded fake-host149passed0failed | PASS |
| Production appliance build/boot | real appliance prerequisites | signed dependency pool/manifest and disk headroom absent; not run | deferred laboratory acceptance |

Integration tests intentionally skip in unchanged quick with NGFW_INTEGRATION unset. No real production image/boot claim. Final git rev-parse HEAD remained exact tested SHA and git status --short returned no output. This proves the frozen source tree; final latest-main D112 integration tree still needs manager gate.

Verdict: PASS.


Correction: full gate PASS above covers bf9fa4b2 only. Product guard correction6bd58ecbe10efb45a486139777655c6c826093cd has independent13PythonPASS3.705s (R1 command/log), but new unchanged complete quick PENDING. Do not treat old source PASS as certification of this new product tree.


Corrected exact-tree unchanged full quick START2026-10-05 08:28:15UTC, session7897, reviewer warm reusable fixture /root/ngfw-wt/r1-gate-hardening-20261005. Exact6bd58ecbe10efb45a486139777655c6c826093cd / tree8d9829d754b58b40842ebff74a30641acda1dee9 and full starting status clean verified. Same bounded loose/TMPDIR heavy command as previous gate; no checks/assertions changed. Raw log images-corrected-quick.log. RUNNING, no new PASS claimed.


Final corrected-tree verdict: PASS. Independent unchanged full quick on exact6bd58ecbe10efb45a486139777655c6c826093cd / tree8d9829d754b58b40842ebff74a30641acda1dee9, started08:28:15UTC, ended08:50:59UTC, exit0/wall22m44s. Warm reviewer fixture /root/ngfw-wt/r1-gate-hardening-20261005, same bounded loose/TMPDIR command above. Contract/install/gitleaks/host guards/slots/generation PASS; TS35/35 (28cached)1m10s, agent lint/race/build12m59s, CLI16s, topology1m02s, images wrapper13tests PASS1.813s; full149fake-host assertions passed across4shards5m26s. Unlike prior P11/upgrade, final harness actually reran on current-main product tree. Unbound race PASS2.041s with safe107-byte socket pathname under reviewer TMPDIR. No source/working-tree warning; exact HEAD/tree and full empty final status rechecked. Archived refs/archive/review-r1/images-corrected-6bd58ecbe. Raw log images-corrected-quick.log and CI steps /root/ngfw-wt/logs/ci/r1-gate-hardening-20261005-20261005-082815-2225364. Production appliance build/boot/signed dependency pool laboratory limits remain unchanged and explicit; no real appliance claim from fixtures.
