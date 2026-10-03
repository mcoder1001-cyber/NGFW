# Independent R2 security and T1 focused verification

Reviewer/tester: root, no product edits.
Frozen local52777be38518a25a0cbc05c3d9f5c6d5f202b149; tree7a092da41be690963ed061e7a6c0553bd4184d90; remotee0d8dcf17d2164cae531e39b35b1f76f655873a3.

R2 APPROVE, no findings. The test-only correction uses the existing private stateDir/hostDirOf mapping and exactly the canonical empty DHCP/QoS projection with protobuf equality. It preserves configured-service removal, management=nil and rendered Unbound checks. No privilege, dependency, authentication, secret, workflow or production delta. Independent R4 confirms private fake-host scope.

Actual independent commands/results:
```text
gitleaks git --redact --config .github/gitleaks.toml --log-opts=a237827811a4abd2293157d5e8ea0cebf61196fc..HEAD
5 commits scanned; no leaks found; exit0
git diff --check a2378278 HEAD
exit0
GOTOOLCHAIN=local go test -race ./internal/agent -run '^TestHostServicesApplyRetrieveRollback$' -count=1
ok ngfw/agent/internal/agent 1.789s; exit0
```

T1 focused PASS; full gate separately required. Fresh R1 APPROVE selected race1.930s and repeat1.802s; R7 APPROVE evidence/scope; R4 APPROVE product-sourceb4ef (product diff unchanged at final frozen source). Hosted source run37116341980 SUCCESS14m22 on exacte0d8. Reviewed history archived remotely archive/go-hostservices-reviewed-20261003. D112 single-main-parent integrationcc0031e2aee7f993801d806ea806b5f228f87353, exact same tree, PR110 mandatory quick37117179841 pending. No merge or full lab/release approval claimed.
