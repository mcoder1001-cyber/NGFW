# F-bruteforce-detectors — R2 security review

Reviewed exact head: `1df528d4ec77c1e7c9b238b26f34da8cd2e811b0` (product checkpoint `5c88cb5f2b09f59a161dcfa6d72cf89b831c207a`) against `main`. Final delta contains only the R5 report, with no product changes. Independent reviewer; no product edits.

Findings: no remaining BLOCKER, MAJOR, MINOR or NIT security findings.

The preliminary input-budget concern is resolved: scan evidence is capped at 10,000 sources, 4,096 ports per source and 100,000 total observations; capacity pruning runs at most once per second. Unsupported thresholds report unavailability instead of inventing a block. Saturated budgets drop evidence, so enforcement under saturation is explicitly limited.

Host observations keep kernel-attested provenance, fresh timestamps and bounded cursor replay protection. Product runtime accepts SSH/kernel scan records, excluding foreign charon logs. Native VPN failures retain configured endpoint/profile matching and bounded SA replay deduplication. Fixed journalctl argv and bounded runner output preserve the shell boundary. Destination ports require canonical numeric 1..65535 values; missing legacy ports cannot count. Canonical/mapped allowlisted management sources are checked before detector windows and again by the agent. No new auth/socket boundary or dependency.

Verification executed:

```text
cd /workspace/scratch/e4f791ef53f7/detectors/apps/agent
/workspace/scratch/e4f791ef53f7/go/bin/go test ./internal/detectors ./internal/agent -run 'AutoBlock|Journal|Host|Native' -count=1
ok ngfw/agent/internal/detectors
ok ngfw/agent/internal/agent

cd /workspace/scratch/e4f791ef53f7/detectors/apps/api
pnpm exec vitest run src/features/auto-block/port-window.test.ts src/features/auto-block/detector-events.test.ts
Test Files 2 passed (2)
Tests 8 passed (8)
```

Gitleaks 8.30.1 with repository `.github/gitleaks.toml` on all changed tracked files at the reviewed product SHA: exit 0; 75.74 KB; no leaks found. No live host/VPP acceptance was executed by R2 or implied by unit evidence.

Verdict: **APPROVE**.
