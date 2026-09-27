# Task: F-bruteforce-block — Brute-force and scan detection with automatic blocking   (prepend 00-CONTEXT.md)

## Goal
Protect the management plane (web UI/API, SSH, VPN logins) and the box itself from password guessing and port scans by
blocking offending source IPs automatically for a while.

## Inputs to read first
- `F-global-blocking` (the enforcement engine — reuse it), P07b/F-aaa (login events), `F-host-acl-nftables` (local-in),
  strongSwan logs, syslog, alarms.

## Contract changes
`security.autoBlock { enabled, rules: [{ source: webLogin|ssh|vpnAuth|portScan, threshold, windowSec, blockSec (escalating
on repeat), }], allowlist: ip-prefix[] (management networks are never blocked), maxEntries }`.

## Scope — build exactly this
1. **Detectors**: failed web/API logins (API side), SSH failures (journald), IKE/EAP auth failures, port scan to local-in
   (nftables counters/log: N distinct ports in window).
2. **Action**: add the source to a dynamic, system-owned Global Blocking list ("auto-block") with a TTL; escalate block time on
   repeat offences; no config commit per block. The allowlist always wins; a lock-out of the admin's own IP is impossible.
3. **UI**: blocked IPs with reason, hits, expiry; unblock and "add to allowlist" actions.
4. **Events**: block/unblock events to alarms/syslog (F-notifications when merged).
5. **Tests**: detector unit tests; topology test: 10 bad logins from a client → blocked on local-in and through; expiry → unblocked;
   allowlisted IP never blocked.
6. **Docs**: `docs/user/security/auto-block.md`.

## Acceptance (paste the evidence)
- [ ] Brute-force from a test client blocked within the threshold, unblocked after blockSec (pasted)
- [ ] Allowlisted IP never blocked (pasted)
- [ ] `tools/ci.sh --base main` green

## Out of scope
IPS signatures (BL-SEC-08), honeypots (BL-SEC-19).
