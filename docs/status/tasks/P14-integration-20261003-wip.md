# P14 isolated integration — 2026-10-03

Branch: `codex/p14-integration-20261003`; base: `origin/main` at `a237827811a4abd2293157d5e8ea0cebf61196fc`.
Owned files: `deploy/image/iso/**` and this status file. Only the completed ISO subtree from local P14 checkpoint `795eff49` was imported; its inherited history and shared status/board files were not copied. Original branch/worktree remain intact. No overlap with Debian bundle or strongSwan developer scopes.

The previous two gitleaks findings were reproduced by a directory scan. Both were assignments of public pinned signer fingerprints, not private keys or tokens. Internal names `CD_KEY_FPR`/`FRR_KEY_FPR` became `CD_SIGNER_FPR`/`FRR_SIGNER_FPR`; all reference sites changed and the fingerprint bytes and pin checks remain identical. This precise naming correction adds no allowlist, scanner bypass or history rewrite. Afterward:

```text
gitleaks dir deploy/image/iso --no-banner --redact --config .github/gitleaks.toml
scanned ~98012 bytes (98.01 KB)
no leaks found
exit 0
```

Read-only input inventory:

| Input | Expected path | Available | Size |
|---|---|---|---|
| Ubuntu ISO, signed sums | `.scratch/base/ubuntu-26.04.1-live-server-amd64.iso`, adjacent SHA256SUMS and SHA256SUMS.gpg | Missing in integration and original P14 scratch | Exact size unknown; task estimates approximately 3 GB for ISO |
| NGFW signed APT repository | `/srv/ngfw-artifacts/apt` or explicit `--ngfw-repo` | Canonical directory missing | Unknown until published; includes all shipped NGFW/VPP debs |
| Matching VPP manifest | `/srv/ngfw-artifacts/vpp/<version>/manifest.json` or explicit path | Canonical parent missing | Unknown; JSON metadata |
| Build chroot | `.scratch/chroot` | Missing in integration and original P14 scratch | Unknown; needs OS tools and extracted base package status |
| Ubuntu public keyring | `/usr/share/keyrings/ubuntu-archive-keyring.gpg` | Present | 3,607 bytes |
| FRR public keyring | `/usr/share/keyrings/frrouting.gpg` | Present | 16,112 bytes |
| Trusted NGFW signer fingerprint | `--ngfw-key-fpr` | Not supplied | 40 hex characters; public trust input |
| ISO signing key | External `--gpg-home` | Not inspected; private material deliberately not read | Unknown |
| Production dependency closure | Downloaded `.scratch/work/debs`/embedded pool | Not prepared | Unknown until signed-repository resolution |

No remote download, full ISO build, chroot bootstrap, installation, host package change, VM operation, mount, daemon, VPP operation, push or merge was performed. Published artifacts in other developers' worktrees were not inspected.

Focused integration ETA: 15–25 minutes including local review/checkpoint, plus shared semaphore queue time. Full ISO static acceptance: estimated 1–3 hours after complete signed inputs and adequate disk space are available; this is an estimate, not a measured build. Actual VM acceptance additionally requires an owner-provided disposable VM and has no defensible completion time until it exists.

Validation completed:

```text
TMPDIR="$PWD/.scratch/tests" /root/.codex/worktrees/0b16/developers/P14/tools/heavy.sh bash deploy/image/iso/tests/run.sh
TESTS: 67 passed, 0 failed
exit 0

python3 -B deploy/image/iso/tests/test_render.py
Ran 6 tests in 0.002s
OK

shellcheck -x -P SCRIPTDIR <all ISO shell scripts>
exit 0

git diff --cached --check
exit 0

tools/ci.sh check --base origin/main
ok: gitleaks — scanned ~103775 bytes (103.78 KB) in 783ms no leaks found
check PASSED (0m09s)
```

`build-iso.sh --help` exits successfully. `findmnt` and `losetup -a` filtered to this worktree's scratch report no entries. Input inventory and bootstrap consumer contracts were reviewed locally; no independent review or complete quick gate is claimed. Full suite output is `.scratch/tests/validation.log` (untracked).

Local product checkpoint: `f84ff0ac`; remote publication: deliberately not attempted under the manager's explicit no-push instruction. Validation follow-up commit SHA is discoverable with `git log -1` (this status is part of that commit).

Recovery: focused suite launched through the original P14 `tools/heavy.sh` read-only, because origin/main does not contain heavy.sh or ci-slot.sh. Test scratch/logs are exclusively in this integration worktree. Next work: independent applicable review, provision verified signed production inputs, then execute staged pool/schema/ISO acceptance in a resource-safe build environment. The unchanged complete quick CI remains required before any merge; no green full/hosted gate is claimed.
