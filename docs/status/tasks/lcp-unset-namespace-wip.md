# LCP unset namespace recovery — appliance repair verified

Branch: codex/fix-lcp-unset-namespace. Source base: deployed 4c8d1b247c6b.
Local source checkpoint: 0e3813d73. Remote source checkpoint: 74fffa92db5a7f2e03dbe56361e727ac52147961. Both have tree 48c8190ad29667de64dc4d76321df17953433aec. Published via authorized GitHub connector because CLI push has no credentials. Draft PR: https://github.com/mcoder1001-cyber/NGFW/pull/249. Not merged; integration-tree conflicts and full hosted quick gate remain for integration.
Owned files: envelope. Independent source review APPROVE, including final Unicode control hardening. Reviewer performed read-only review; author ran tests.

## What changed

Malformed namespace binary API readback now requires independent exact unset proof from fixed read-only CliInband `show lcp` on the same VPP client. Named, ambiguous, duplicate, failed, oversized, invalid UTF-8 or control-containing status fails closed. Valid API results are unchanged. No public contract, VPP source, privilege or sandbox changes.

## Actual verification

- Go1.26.0 official archive SHA256 verified: aac1b08a0fb0c4e0a7c1555beb7b59180b05dfc5a3d62e40e9de90cd42f88235.
- `go test ./internal/subsystems -run TestBasePolicy -count=1`: PASS, 0.018s (19 new table cases plus existing projection tests).
- Same focused command with `-race`: PASS, 1.065s.
- `go test ./internal/renderers/basepolicy ./internal/descriptors/lcp -count=1`: PASS, 0.021s / 0.123s.
- `go vet ./internal/subsystems ./internal/renderers/basepolicy ./internal/descriptors/lcp ./internal/agent`: PASS.
- Stripped static agent build: PASS; SHA256 7f5ff341aa5894fcf28de4efaaca5afe40a18cdd6fcbef88d7d51072088a7d7f.
- `tools/ci.sh check --base 4c8d1b247c6b`: PASS. Gitleaks absent locally; built-in secret pattern checks passed. This is not the full quick gate.
- Broader subsystems+agent tests FAIL on legacy fixture directory permissions. Untouched deployed-source baseline independently reproduces `TestRegisterACLWiring`: fingerprint key directory must not be group/world-writable. No baseline test or security check weakened. Full quick gate not claimed.
- Initial test compile failed on a nonexistent namespace-get Retval field; checked generated reply, corrected test, then reran focused tests successfully.

## Appliance result

Backed up installed binary and agent state under `/root/ngfw-lcp-repair`. Installed only matching-source corrected static agent. Explicitly set existing API owner `ngfw`, which also corrects empty-owner auto-block cache serialization; no new owner/privileges. Both services active with zero restarts after installation. VPP MainPID unchanged. Staged candidate payload SHA256 unchanged; staged password/firewall settings were not applied.

Twenty consecutive authenticated read-only validation requests: HTTP200, ok=true, zero namespace errors. Remote receipts: deployment.json, verification.json, recovery.json in `/root/ngfw-lcp-repair`. Existing binary backup ngfw-agent.before and agent-state.before.tar are root-private. Current user draft still requests UTC while provisioned identity is Asia/Tehran; the existing identity-write package limitation is separate and remains explicit.

## Recovery / next work

No further appliance change required for this namespace repair. Preserve user's pending wizard. For integration, port/rebase patch onto current main, resolve integration conflicts, run unchanged full hosted quick gate and obtain review for the integration tree; do not merge this old-base draft directly. Broader package and real Internet acceptance remain unverified. Exact next command for investigation: `cat /root/ngfw-lcp-repair/verification.json` on the appliance.
