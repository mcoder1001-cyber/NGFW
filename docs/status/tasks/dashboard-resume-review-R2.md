# Dashboard resume — independent R2 security review

Reviewer: fresh R2 agent; no product code written. Local-only review branch `codex/dashboard-security-review-20261002`; review worktree `/workspace/scratch/de92de7d9874/NGFW-dashboard-r2`.

## Exact scope

- Main: `53a43ce5b91e71f3fedc282f9c5c54ba22fc9fb8`.
- Product checkpoint reviewed/tested: `6563e1073b76b0e439d19b99731f5715bdf3a838`, tree `d2eaaac3d9877884befbe8560dd8f9505016878b`.
- Frozen resumed head additionally reviewed: `1379424b90ea7b076458f361159c8db80a4cdf66`, tree `92da480054bc059847ee95ab53c983c4565d17d4`. Only two resume status/envelope files differ from the product checkpoint; inspected their exact committed contents.
- Prior recovered remote checkpoint: `148a7cc8a830c76189f0427007561b04174e8e94`, tree `45b99acedf348d715300c60f57049adccfed920a`.

Read AGENTS, shared context, contributing/decision policies, REVIEW-PROMPT, and R2 prompt. Assessed agent projection, desired listener validation, exporter, stats source, subsystem lifecycle, and changed evidence files. Existing schema explicitly defaults external listener disabled, address `0.0.0.0`, empty allowlist permitting reachable peers; implementing this existing contract is not a new security-boundary decision.

## Findings

No confirmed R2 BLOCKER, MAJOR, or MINOR in this delta. Allowlist evaluates socket RemoteAddr rather than forwarded headers; invalid addresses/ports/CIDRs reject, same-address updates replace authorization handler, metrics errors return generic text. No new shell invocation, SQL, secret response, privileged file write, API route, or dependency introduced by dashboard product changes. Stats source is read-only, cancellation prevents post-stop reconnect, and HTTP shutdown closes remaining connections after deadline. The collector emits counters with existing escaped labels.

Historical security approval is supported for carried dashboard product paths by exact byte-equivalence to the recovered remote checkpoint plus this fresh static review. The historical R2 report's original `35c4533e...` object is unavailable locally: direct equivalence to that older SHA is NOT asserted. Changes from recovered checkpoint outside product paths consist of already-main build DAG/package scripts and documentation; this verdict is R2 only, not a substitute for other mandatory panels or integration gate.

## Actual verification

Executed in this isolated review worktree, using `/workspace/scratch/96b8b6fbc8a7/toolchain/bin` for pinned tools:

- `git diff --exit-code origin/codex/dashboard-recovered-20261002 6563e107 -- apps/agent apps/web`: exit 0, no output.
- `git diff --exit-code 6563e107 1379424b -- apps packages deploy tools turbo.json`: exit 0, no output.
- Fallback changed-file secret regex (private-key/password/token shapes): 29 files, no matches. No source execution/shell/SQL/file-write matches from targeted security grep.
- `gitleaks detect --no-git --redact -s apps/agent --config .github/gitleaks.toml --no-banner`: exit 0, scanned approximately 15.46 MB, no leaks found.
- `gitleaks detect --no-git --redact -s docs/status/tasks --no-banner`: exit 0, scanned approximately 5.66 MB, no leaks found. Two later resume documents separately inspected using `git show 1379424b:<path>`.
- Initial scan from apps/agent without repository configuration reported three generic-api-key matches in rsyslog/rsyslog_test.go:31, scheduler/validator_test.go:372, strongswan/apply_test.go:375. All three files are unchanged against main (verified exit 0 diff); repository-configured scan passes. No secret values copied into this report.
- In apps/agent: `env PATH=/workspace/scratch/96b8b6fbc8a7/toolchain/bin:$PATH GOCACHE=/tmp/dashboard-r2-gocache GOMAXPROCS=2 GOFLAGS=-p=2 go test ./internal/promexport -run 'Test.*(Allow|Handler)' -count=1` returned `ok ngfw/agent/internal/promexport 0.012s`, exit 0.

No full quick gate, race suite, real VPP, appliance, or browser acceptance run by this reviewer. Parent owns mandatory full gate and merge decisions. Nothing published.

Verdict: **APPROVE** (R2 code security only, exact frozen head above).
