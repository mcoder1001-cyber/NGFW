# TD19 safe-root source handoff

Branch codex/td19-safe-root-20261007; isolated worktree /root/ngfw-wt/td19-safe-root-20261007. Base f6ae6e555. Frozen reviewed candidate 269c571608e29e6106a93de9b756433f40b0a130, successfully published. Draft PR https://github.com/mcoder1001-cyber/NGFW/pull/204. Own files declared in envelope. No board, main, tools/lab, inventory or other worktree edits.

## Behavior

00/20/40 implement NGFW_INSTALL_ROOT and read-only --dry-run. Alternate-root apply is exclusively a recording fixture: each executable matches the shipped deterministic recorder, unknown regular APT/Go/corepack/pip code refuses before commands; fixture PATH cannot shadow checked tools. Private staging, Go/generator homes, profile, repository output, Python venv and caches use the selected root. The original artifact verifier uses selected-root temporary staging. Initial trusted Python and recorder use isolated mode.

Default-root Ubuntu OS metadata accepts only the standard /etc/os-release link to canonical regular /usr/lib/os-release; fake roots retain all symlink refusals. Codename is literal bounded data (O_NOFOLLOW/O_NONBLOCK, <=64KiB), never evaluated as shell; shell substitution/FIFO/oversize negatives are covered.

D-238 exact FRR/NodeSource primary identities, key validity/capability/secret-material gates, original signed seven-runtime product manifest gate and Go archive SHA/version/generator pins remain enforced. Dry-run is a plan and does not claim remote keys/artifacts were verified.

Build pnpm is exactly package.json packageManager (12.5.1). Lab apply requires NGFW_LAB_REQUIREMENTS: bounded regular administrator-reviewed complete pip lock, all direct lab dependencies including pip, exact versions and SHA256 hashes, no URL/options or duplicate dependencies. The validated lock snapshot is used with isolated pip require-hashes; resolver requires full transitive closure at apply. No authoritative complete lab lock exists in the repo: default apply refuses before APT. Synthetic versions/hashes are fixture inputs, never release pins.

## Actual verification and historical failures

New/native fake-root recording suite 11/11 PASS10.762s on product sourcebd0ec236f (scripts tree identical through final269); includes native metadata pure read-only helper, bad Go digest before tar, exact pnpm fallback, rooted hashed venv, D-238 refusal, missing/nonrecording tool and rooted Go/GOBIN/pip sentinel negatives. Syntax, ShellCheck and diff check passed. Final exact269 complete strict46 fixture run PASS149.121s: tests46, failures0, errors0, skipped0, expectedFailures0, unexpectedSuccesses0. Independent final269 reviewer APPROVE, published4c50ca949626068be417c3522110abad370357f1: native11PASS10.312s, preflight7PASS54.720s, original Go/APT negative2, isolated Python startup3, OS metadata injection1 and syntax/ShellCheck/source identity checks PASS.

Earlier unchanged complete strict45 on664 PASS148.297s zero failures/errors/skips; strict46 on863 PASS153.190s. Earlier new9 exposed swallowed Python path refusal in command substitution (fixed); first strict45 failed21 copied-entry/extracted-fragment assertions (seam fixtures adapted without weakening adverse trust assertions); next strict45 failed1 missing-helper repo-keys copy (fixed). Independent review rejected545 regular-stub/rooted-executable authority bypasses: repaired with shipped recorder and tested sentinel refusals. It later found native OS metadata symlink rejection: repaired with read-only canonical fallback and native regression. No source failure was waived/deferred as lab acceptance. Full aggregate CI was deliberately not run under current owner waiver.

## Remaining / scope

Whole TD19 is not DONE. Release Python lab lock/closure needs authoritative reviewed pins and real resolver/install acceptance; real target APT, signed artifact provisioning, installation/boot and hardware acceptance were not run. Inventory/provision source belongs to separate owner. No APT/install/network/package/service/VPP mutation occurred: effects recorded under disposable roots, private fixture GPG certificates only. Bounded source and independent review complete; no current source failure remains. Manager integration/merge is pending. Manager owns integration/rebase/current-main validation/merge and archival of published coherent history.

Checkpoint SHAs and actual publication are recoverable from git log and git ls-remote origin refs/heads/codex/td19-safe-root-20261007. Original failed history is retained. Exact next manager commands: git fetch origin codex/td19-safe-root-20261007; git show origin/codex/td19-safe-root-20261007:docs/status/tasks/td19-safe-root-20261007-report.md. Import final report-only receipt; preserve reviewed history before D112 squash and expected-head merge. Developer freezes branch after this receipt and will not push without manager coordination.
