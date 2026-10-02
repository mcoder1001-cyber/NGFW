# Multi-WAN monitor slice — independent R2 security review

Reviewed PR65 remote `987c2c9f1511b8694c5332783a9fd7eafdae8ae2`, product tree `b7c62b157300d9f588a2c5784685480eb1044004` (local `4a7b0b50`), plus documentation checkpoint `7e211100`. Base `31355cef`. Isolated reviewer branch `review/wan-monitor-r2-20261002`, worktree `/workspace/scratch/de92de7d9874/ngfw-wan-review-r2`. Owned file: this report only. Read AGENTS, shared context, contributing/decision policy, REVIEW/R2 and current/inherited WIP envelopes. No product edits.

## Findings

No BLOCKER, MAJOR or MINOR security findings in this bounded monitor-only change.

- Every production dial uses SO_BINDTODEVICE; socket-control/binding failures return errors. The Go resolver uses the same bound dialer; there is no shell, proxy environment use or unbound fallback. Missing LCP, nondefault VRF and nonempty namespace are rejected. Device lookup reads service-owned stored desired interfaces under its mutex.
- HTTP uses fixed HTTP/80 HEAD, 16 KiB response-header cap, no redirect following, no response-body consumption and request-context cancellation. Targets cannot inject paths, credentials, arbitrary ports or CR/LF. DNS is a connected UDP/53 exchange with a 4096-byte receive cap, transaction/question/response/truncation/status checks. ICMP uses IPv4, bounded reads, matching echo ID/sequence/payload, cancellation and deadlines. These health protocols are observations, not cryptographic peer authentication.
- Runtime bounds workers to 2048, intervals to 100–600000 ms and individual probe deadlines to 50–60000 ms; workers do not overlap themselves. Replacements cancel/drain and reject stale-generation results. Malformed probe metrics fail closed. State snapshots contain no secrets.
- WanState checks the existing owner policy before serving snapshots; existing REST wrapper remains protected. No new write endpoint, auth model, socket permissions, dependencies or privilege change. Watcher reads durably stored state and invalidates health when interface identity changes.
- Active remains unset. There is no forwarding route, NAT cleanup, VRF controller or ABF mutation in this patch; user documentation and projection warning explicitly preserve that boundary.

## Reviewer verification

Static inspection of all changed source and documentation, existing owner policy, stored-interface refresh and protected REST wrapper. `gitleaks` was unavailable; fallback scan applied private-key/password and shell-execution patterns to every changed file using Python regular expressions:

```text
Changed-file fallback secret/shell scan: 13 files; 0 matches
```

```text
git diff --check 31355cef..7e211100
(exit 0, no diagnostics)
```

Additional reviewer execution:

```text
bash tools/ci.sh check --base 31355cef
WARN gitleaks not installed — built-in secret grep only
ok: 30 developer slots + CI slot 12; 964 ports, 32 id ranges, no collision; tools/lab env verified for 31 slots
check PASSED (0m02s)
```

No broad duplicate quick gate or product test run by this reviewer. Developer-focused race/vet evidence is recorded separately and is not claimed as reviewer execution. Hosted unchanged quick remains an independent merge requirement. Real Linux binding, raw ICMP/network behavior, VPP and browser/lab acceptance NOT RUN here; source inspection and fake-peer tests do not certify those.

Verdict: **APPROVE** (R2 security, monitor-only scope).

Publication: report checkpoint is local pending manager-coordinated GitHub publication; do not report a remote SHA until publication succeeds.
