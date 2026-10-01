# CI-remote-codex — R1 correctness and tests

Reviewed commit `131c3aaf88a6b56603ba9c7cb48916877b14a830` against `origin/main`. Scope: `.github/workflows/ci.yml`; manager board checkpoints excluded.

## Findings

No BLOCKER, MAJOR or MINOR correctness findings.

- The workflow invokes the original `tools/ci.sh quick` directly, without missing-tool allowances, reduced modes, or skipped phases. The original quick path includes installation, generation dirty gate, contract guard when a base is supplied, forbidden patterns, turbo lint/typecheck/test/build, agent, CLI, test modules and deployment checks.
- Full checkout history makes `origin/<PR base>` and `origin/main` available. PR checks use their actual target branch; non-main pushes and dispatch use origin/main. Main runs the unchanged mandatory quick gate without a branch contract comparison (comparison against its own origin/main would be empty).
- Node/pnpm/Go/buf and protobuf generators are pinned. Generated Go headers confirm protoc-gen-go v1.36.12; buf local generator configuration consumes the installed plugins. The pnpm frozen-lockfile gate pins the TS generator dependencies.
- Bash uses `set -euo pipefail`, array arguments with quoted base refs, fail-on-HTTP-error downloads, checksum verification before executable installation, and ordinary non-conditional invocation of the quick gate. Failures propagate to the job. The checksum pipeline also fails if no matching checksum exists.
- This infrastructure change does not require live VPP or alter product code. Actual hosted execution remains necessary before claiming the hosted gate is green.

## Independent commands and evidence

Worktree `/workspace/scratch/96b8b6fbc8a7/NGFW-ci`; PATH includes `/workspace/scratch/96b8b6fbc8a7/toolchain/bin` and `/usr/local/go/bin`.

```text
$ actionlint .github/workflows/ci.yml
[no output; exit 0]
$ tools/ci.sh check --base origin/main
no contract files changed in the 3 commit(s) of HEAD since origin/main (5561d041)
ok: no shell/VPP/FFI access in apps/api/src apps/web/src packages/*/src
ok: no Dockerfile/compose files
ok: no kill-by-pattern in scripts
ok: no secret-shaped strings
ok: vrxtestsecrets only in test code
ok: gitleaks — scanned ~14677 bytes (14.68 KB) in 170ms no leaks found
ok: no packet trace (trace add / show trace / clear trace / tracedump API) outside docs and the generated bindings
ok: 30 developer slots + CI slot 12; 964 ports, 32 id ranges, no collision; tools/lab env verified for 31 slots
check PASSED (0m03s)
```

An independent `tools/ci.sh quick --base origin/main` was started and reached frozen-lockfile installation after passing contract/tool checks. On manager instruction it was cancelled (exit 130), avoiding a repeated full run on the unchanged environment already known to deny Unix sockets. This reviewer does **not** claim a full quick pass or independently reproduced EPERM; the developer report documents BLOCKED-ENV. GitHub-hosted execution is pending and must be recorded separately.

Verdict: **APPROVE** for workflow correctness. Full quick/hosted tester evidence is pending, not PASS.
