# F-bfd-redistribution — independent R2 security review

Reviewed source: c6c4247da3293a152ed0327377c12cf4cf44f2e2. Independent reviewer; no product edits. Diff reviewed against merge base origin/main.

No BLOCKER, MAJOR or MINOR findings. Reviewed protected read-only REST routes and owner-checked agent RPCs, existing sealed secret delivery/cache, CA signing-key exclusion, BFD key-kind and 1–20-byte limits, daemon identifier validation, fixed FRR commands and argv-only execution. Secret refs are distinct from secret material; desired values, events and state responses contain no raw keys. The source adapter returns a copy, descriptor clears that copy and request key bytes, and resolver errors are masked with ErrNoSecret instead of interpolating arbitrary secret-bearing diagnostic text. No new authentication/socket/privilege boundary.

Commands executed in /root/ngfw-wt/ready-bfd-20261005 with TMPDIR=/root/.cache/review-r2:

```text
GOMAXPROCS=2 GOFLAGS=-p=2 tools/heavy.sh go -C apps/agent test ./internal/descriptors/bfd ./internal/desired ./internal/agent -run 'Test.*(Bfd|BFD|Secret)' -count=1
heavy: slot 1 after 150s
ok ngfw/agent/internal/descriptors/bfd 0.020s
ok ngfw/agent/internal/desired 0.140s
ok ngfw/agent/internal/agent 0.476s
```

`gitleaks detect --no-git --source docs/status/tasks --config .github/gitleaks.toml --redact --no-banner`: exit0, no leaks. A separate disposable copy of all changed existing files, including generated outputs, scanned with the same configuration: ~6.86MB, exit0, no leaks. Commands did not mutate shared host services or VPP. Full gate and runtime acceptance are separate.

Verdict: APPROVE.
