# T1 independent complete quick gate — PASS

Exact source 4c8640695c8ac9877819817506979e361e801216; tree 79c94d41e9a5e0fc71b7a8837ed9956d661258fd. Own branch codex/f-backup-correctness-final-review-20261005. Worktree remained clean and source frozen throughout. Includes main f57424992 security dependency freeze. Actual complete process exit 0.

```sh
env NGFW_CI_TASK_CONCURRENCY=2 GOMAXPROCS=2 GOFLAGS=-p=2 NGFW_CI_LOG_DIR=/tmp/fbr-correctness-integration-final-ci tools/ci.sh --base origin/main
```

Actual terminal excerpt:

```text
Tasks:    35 successful, 35 total Cached:    18 cached, 35 total Time:    9m59.401s  
== apps/agent: make lint test build ==
== apps/cli: make lint test build ==
== test/ Go modules, unit mode (test/integration/reachability test/integration/smoke test/topology/ab-upgrade test/topology/acl test/topology/backup-restore test/topology/bonding test/topology/bridge-l2 test/topology/det44 test/topology/ha-state-sync test/topology/hardening-lite test/topology/host-acl-nftables test/topology/images test/topology/interfaces test/topology/ipfix-sflow test/topology/kea-dhcp-relay test/topology/loopback-bvi-gso-lldp-span test/topology/nat44-ed-sessions test/topology/nat44-ei-64-66-nptv6 test/topology/neighbors-ra test/topology/object-model test/topology/qos-flat test/topology/system-identity test/topology/traffic-a/globals test/topology/traffic-c/globals test/topology/unbound-chrony-syslog test/topology/vlan-qinq test/topology/vrf-static-ecmp) ==
== deploy/vpp: shellcheck + apply-startup fake-host harness ==
shellcheck ok: ./apply-startup.sh ./build.sh ./lib.sh ./test-apply-startup.sh ./verify.sh
apply-startup harness: unchanged since a green run (2026-10-05T17:27:13+00:00 harness green (4 shards)) — skipped (key 27a79729b9ac; rm /root/.cache/ngfw-ci/apply-startup/27a79729b9ac200ded1ecb63547bf09b5a4b14bf4f97fa90c2c2382fe9baa964 to force)
  mode quick · wall time 21m16s · logs /tmp/fbr-correctness-integration-final-ci/f-backup-correctness-review-20261005-20261005-172751-319761
CI GATE PASSED
```

Full output /tmp/fbr-correctness-integration-final-ci-output.log; step logs /tmp/fbr-correctness-integration-final-ci/f-backup-correctness-review-20261005-20261005-172751-319761. Gate code/options unchanged; no omitted stage. Deploy fake-host harness used the unchanged gate's own green cache. Earlier full old-base run37c56a83 executed its four actual shards33+29+26+61 cases, zero failures, then CI GATE PASSED34m29s; unchanged harness hash27a79729b9ac200ded1ecb63547bf09b5a4b14bf4f97fa90c2c2382fe9baa964. Thus do not describe final cached phase as a fresh harness rerun. Go topology modules run unit mode; lab integration deliberately skips unless NGFW_INTEGRATION set, as printed by unchanged gate.

Historical runs: initialc4ad failed exact navigation fixture1/619 web tests, corrected095;493 interruptedTERM143 during generation on manager R3 hold, never PASS. Old-base37c fullPASS is superseded by this fresh-main exact integration PASS. Hosted final squashed PR gate is manager's subsequent responsibility. Verdict: **PASS**.
