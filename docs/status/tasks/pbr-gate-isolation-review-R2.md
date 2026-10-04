# pbr-gate-isolation — R2 security review

Reviewed frozen head: `009bdda0dd9be21799832ce33d08f4c7c8990a17`, baseline `6eda5e94acca007bd290e37ab74d53ba45157e30`. Independent read-only product review.

Findings: no remaining BLOCKER, MAJOR, MINOR or NIT security findings.

Conditional ACL projection uses both prospective and stored configuration. An unrelated interfaces transaction with no ACL, no enabled AutoBlock and no runtime entries no longer authorizes deletion of owner-tagged external ACLs. Existing explicit ACL configuration, prospective enabled overlay, formerly enabled overlay (including disable/removal with no entries), and cached runtime entries still expand ACL scope. Thus disable/removal clears stale VPP/host enforcement; adding interfaces under active blocking retains bindings. Metadata persistence remains scoped to original authoritative request domains.

Apply and planSources use the projected effective scope; DryRun, drift checks and dynamic-source plans share that plan path. Dynamic source scope additionally preserves its existing descriptor isolation. No descriptor ownership, foreign-object refusal, ACL tag, socket privilege or authentication boundary was changed. Fixture isolation changes only local test registry construction and retains behavioral checks.

Executed verification on final product content:

```text
cd /workspace/scratch/e4f791ef53f7/gate-fix/apps/agent
/workspace/scratch/e4f791ef53f7/go/bin/go test -count=1 ./internal/agent -run 'TestInterfaceScope|TestDisabledAutoBlock|TestAutoBlockRuntimeLifecycleOnFake'
ok ngfw/agent/internal/agent 0.147s
```

Earlier broader AutoBlock|GlobalBlocking|ACL|Acl focused tests passed (1.993s) before the final predicate refinement. New regressions exercise retained external ACLs, active-overlay interface binding, whole AutoBlock removal, and disabled context isolation; existing lifecycle test verifies security-only disable and confirm revert.

Pinned Gitleaks 8.30.1 with repository configuration on copied changed and untracked files: exit 0, 143.11 KB, no leaks found; subsequent predicate/test delta includes no secret material. No complete CI or live VPP/host acceptance is claimed.

Verdict: **APPROVE**.
