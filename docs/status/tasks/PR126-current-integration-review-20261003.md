# PR126 current-main integration review — 2026-10-03

Reviewed immutable PR sourceee288a57f20daac820aff8ae90084fcf1ae1959f versus current main360018d3bc190e23e7a54815f931827ed85fdd90 using targeted Git metadata/new paths. Own review branchcodex/pr126-current-review-20261003, isolatedPR126-review based360018d3. No author worktree/product edits, merge, heavy/full tests or host operations.

Verdict: PR126 source is reviewed against an older main composition; DO NOT merge its old green result as proof of the current combined tree. Preserve rename implementation and integrate the bounded newer main delta, then obtain fresh checks on that exact tree. Existing author followup worktree metadata shows a59f75edd OSPF composition work, but that is not the audited PR head and is not certified here.

## Freshness and inherited coverage

PR head parent is dcaab0327b0669cd207689e939459921f33c019f. That parent already contains merged PKI RPC123, console122, precise PKI expiry124 and ISO disk-size125. Their source is not missing from this PR base; renamed PKI/size consumers still require composed-tree tests, not reimplementation. Prior independent source approvals cover bounded RPC unavailable semantics, precise expiry and installer guard/count corrections. Existing rename worker WIPs record canonical proto/generation, agent/unit, SDK and packaging fixture checks, and mandatory fresh integration/architecture reviews; those are component/history evidence, not independently rerun current-head results. Coordinator reports source-only405 green on the older tree; no new CI pass is claimed by reviewer.

Current main adds13 changed paths after dca: OSPF127 contributes new six API feature files, app.module.ts registration, generated CLI/TS operations and two WIPs; license129 adds generate-license.sh and25guide lines. No broad2k-file diff was needed to identify that delta.

## Required current integration

1. Preserve complete read-only OSPF route/DTO/parser/compatibility tests and exact controller registration alongside renamed module composition. New tests retain https://ngfw.dev/problems/unavailable while renamed common/problem.ts uses https://ngfw.dev/problems/. Align those expected protocol identifiers and new fixture prefix conventions with the renamed source/guard rules. Package imports remain @ngfw and do not need speculative renaming. Old PR generated clients have no Ospf_state/stateospf operation. Regenerate from the actual combined OpenAPI through repository generators: TS client, Go CLI operation table/reference and Python SDK; verify Terraform/canonical SDK output via existing generation check rather than copying older generated snapshots.

2. Adapt new Bash license generator to actual renamed issuer. Current wrapper CLI points at ngfw-license.mjs, deleted by PR126 in favor of ngfw-license.mjs; its existing key paths default ngfw-license-signing/public.pem while renamed keygen writes ngfw-license-signing/public.pem. It emits NGFW_LICENSE_PUBLIC_KEYS but renamed API consumes NGFW_LICENSE_PUBLIC_KEYS, preventing generated-key installation. Align wrapper configurable environment names/default paths/help/output extension and newly added licensing guide paragraphs with the renamed API/issuer. Preserve the generator's external key custody, override/expiry logic and shell-safe public-key export. Do not expose credentials or silently change trust.

3. Resolve the shared app.module/generated-client/licensing-guide composition against the exact current base, coordinate with active rename/license owners, and regenerate. Avoid old branch merges that replace newer feature/domain content. Freeze and publish a new exact-tree checkpoint before final CI. Historical refs/actual tests remain evidence; do not relabel old runs as new results.

## Required fresh validation and limits

Run unchanged complete hosted quick gate on the final current composition, existing contract/wire/namespace and generated consistency checks, bounded OSPF source tests plus API typecheck/CLI operation freshness, full affected SDK checks, and license-wrapper offline invocation coverage with the renamed issuer/default files/API env variable. Secret key fixtures must live outside checkout and outputs must not print private material. Existing author-reviewed namespace/security/geometry/generator corrections retain applicable source review; request fresh applicable integration review for newly composed OSPF/license behavior and any actual fixes rather than blind approval.

Actual new renamed Debian VPP/NGFW package builds, inspected artifact identity/provenance and any resulting appliance release acceptance remain NOT RUN/unverified in this review. Historical packaging fixtures or an older genuine artifact do not certify new renamed binary outputs. PKI materialization/P11 secret transport and live lab acceptance remain externally owned/incomplete. No board task should be marked fully complete solely from renamed source or older405 tests.
