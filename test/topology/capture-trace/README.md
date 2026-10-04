# Capture acceptance

These checks distinguish transport verification from real dataplane evidence. Unit-mode compilation or a skipped Go host test never proves packet capture.

## API transport, authorization and audit

On a provisioned worker slot with PostgreSQL and Valkey, run:

```bash
NGFW_TEST_PREFIX=w5 NGFW_VALKEY_DB=5 NGFW_INTEGRATION=1 pnpm --filter @ngfw/api exec vitest run -c vitest.e2e.config.ts test/e2e/capture-trace.e2e.test.ts
```

The shared API harness uses the slot database and its fake agent. It tests real guards, audit persistence and binary gRPC/HTTP download; the pcap itself is fake.

## Persisted recovery on VPP

Without the dedicated socket opt-in, the host test explicitly skips (NOT RUN); configuring the shared socket fails. Dispatch capture is banned on shared VPP (D-128). Provision a dedicated per-slot VPP and run one host package at a time:

```bash
NGFW_TEST_PREFIX=w5 NGFW_CAPTURE_DEDICATED_VPP_SOCKET=/run/ngfw-test/w5/vpp/api.sock NGFW_INTEGRATION=1 go -C apps/agent test -v -count=1 ./internal/agent -run '^TestCaptureInterruptedRecoveryOnHost$'
```

This creates a slot-owned loopback and capture, persists its running record, re-opens the boot store and recovers as a restarted agent. It verifies same-boot stop, interrupted metadata, temporary file cleanup, retained file permissions when VPP wrote one and DELETE. It does not restart the VPP process or emit traffic. Capture busy is a failure requiring a quiet lab window, not a passing skip. Record `systemctl show vpp -p NRestarts` before and after; stop acceptance if it changes.

## Actual rig packets

Start an isolated API and agent on the worker slot; retain their PIDs for cleanup. Use a slot rig wired to the dedicated VPP (do not use the shared VPP rig) and a separately built `ngfw-vpp-preflight`. Set the bearer token only in `NGFW_CAPTURE_TOKEN`, never in argv, a committed file or evidence. Example for slot 5:

```bash
NGFW_TEST_PREFIX=w5 python3 test/topology/capture-trace/live.py \
  --dedicated-vpp --vpp-unit ngfw-test-w5-vpp --vpp-socket /run/ngfw-test/w5/vpp/api.sock \
  --api http://127.0.0.1:3500 --interface host-w5l0 --peer 10.5.2.2 \
  --capture-dir /run/ngfw-test/w5/captures \
  --preflight /run/ngfw-test/w5/bin/ngfw-vpp-preflight \
  --output /run/ngfw-test/w5/capture-evidence
```

The runner holds the shared lab lock and a dedicated host-capture serialization lock. Before fixed-argv ping from `ns-w5-lan`, TD-3 preflight must pass. It checks 400 `/bpf`, 409 busy and active DELETE, real packet count, Stop completion, 0600 single-link capture, `/tmp` cleanup, downloaded hash and size, `tcpdump -r`, DELETE and 404. It writes `evidence.json` with actual command results and shared and dedicated unit NRestarts before/after; it never logs the token. On failure it requests stop only for its own API stream. Inspect slot state after failed runs; do not blindly remove other captures.

The explicit dedicated-VPP acknowledgement must describe actual observed provisioning; never use it to bypass the shared-host ban. For BPF append `--bpf icmp` only in an approved globals-owner manager window with `NGFW_DF8_GLOBALS=1`. The runner additionally holds the globals lock; the production manager restores its config-owned filter. Record before/after globals configuration separately because those VPP globals have no readback API. Never enable packet trace on shared VPP. Screenshot acceptance remains a manual T4 capture of the actual Tools page and evidence download; no synthetic screenshot counts.

Local runner checks: `python3 -m unittest discover -s test/topology/capture-trace -p 'test_*.py'`.
