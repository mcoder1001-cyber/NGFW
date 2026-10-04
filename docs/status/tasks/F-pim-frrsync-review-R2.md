# F-pim-frrsync — R2 security review

Reviewed exact product head: `859638f03a25e97e6f0ca9c2d2418df42d1d3efe` against `main`. Independent reviewer; no product edits.

Findings: no BLOCKER, MAJOR, MINOR or NIT security findings.

- FRR commands are fixed argv values; RP addresses and multicast prefixes are parsed and rendered canonically, and LCP host interface names use the framework alphabet validator. No untrusted strings enter a shell.
- FRR observations are limited to 4 MiB and 10,000 routes; addresses/path flags are validated, and unknown or ambiguous LCP interfaces suppress installation. Reader failure retains the last validated snapshot.
- Named descriptor boot records separate static and PIM ownership. Create rejects an existing foreign route; retrieve/delete require the matching named record and current VPP boot. Numbered lab owners cannot install global-table PIM state.
- No new API/auth/socket boundary, dependency, secret-bearing state or persistence format.

Verification executed:

```text
cd /workspace/scratch/e4f791ef53f7/pim/apps/agent
/workspace/scratch/e4f791ef53f7/go/bin/go test -count=1 ./internal/frrsync/pim ./internal/renderers/frr/pim ./internal/descriptors/mfib
ok ngfw/agent/internal/frrsync/pim
ok ngfw/agent/internal/renderers/frr/pim
ok ngfw/agent/internal/descriptors/mfib
```

Gitleaks 8.30.1 with `.github/gitleaks.toml` scanned all changed tracked and untracked files copied to `/tmp/ngfw-r2/pim`: exit 0, 74.48 KB, no leaks found. Final delta from prior scan adds only tests using public documentation addresses and status evidence. Live VPP/FRR integration was not executed by R2 and is not implied by unit evidence.

Verdict: **APPROVE**.

Final delta verification: 256-route supported runtime snapshot limit rejects overflow before replacing cache. `go test -count=1 ./internal/frrsync/pim` executed at final source and passed (0.031s), including boundary/cache preservation regression. No security boundary weakened.
