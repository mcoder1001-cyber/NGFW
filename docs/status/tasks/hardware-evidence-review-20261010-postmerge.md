# Independent R7 postmerge and recovery evidence addendum

Reviewed public manager checkpoints:
`925703c2a02c2b7e15e28e0ddbbd4534bca19de3` and final recovery checkpoint
`0f3ab280f3d3941b21e2580d584bdaddc614822c`.
Reviewer `/root/evidence_review`, own worktree/branch
`/root/ngfw-wt/hardware-evidence-review-20261010`,
`codex/hardware-evidence-review-20261010`. Only owned review docs changed.
No target commands, product changes, backup writes/extraction to disk, service
actions, repair, reboot or duplicate full quick were performed by this reviewer.
Inspection timestamp: 2026-10-10 07:56 UTC.

**Verdict: APPROVE** the public postmerge/recovery evidence at exact
`0f3ab280f3d3941b21e2580d584bdaddc614822c`. No new R7 findings. Original source
APPROVE on5bd7e8b remains separate. This addendum does not certify hardware
acceptance, complete data backup or the still-running bare main quick.

## Exact merge and CI claims

Commands ran in the reviewer's own worktree:

```text
gh pr view 217 --json state,mergedAt,mergeCommit,headRefOid
state=MERGED; mergedAt=2026-10-10T07:46:33Z
headRefOid=5bd7e8b545fc765fd2babd8dda15175d6f33af1b
mergeCommit=4908716b4501312102382e6979b8fc1ded6f9311
git show -s --format='%H %T %P %s' 4908716b4501312102382e6979b8fc1ded6f9311
4908716b4501312102382e6979b8fc1ded6f9311
tree=a0d7b7cbc7f37dc2fadd9a93d41ce486dcda50d2
parents=d2d55984d74fa1d06c32e8271886f11f16375407 5bd7e8b545fc765fd2babd8dda15175d6f33af1b
gh api repos/mcoder1001-cyber/NGFW/git/commits/8a15d644c53cc3ef4abde339efd3b2a0331221a5 --jq '{sha: .sha, tree: .tree.sha, parents: [.parents[].sha]}'
{"parents":["d2d55984d74fa1d06c32e8271886f11f16375407","5bd7e8b545fc765fd2babd8dda15175d6f33af1b"],"sha":"8a15d644c53cc3ef4abde339efd3b2a0331221a5","tree":"a0d7b7cbc7f37dc2fadd9a93d41ce486dcda50d2"}
gh run view 38033766837 --json status,conclusion,headSha
{"conclusion":"success","headSha":"5bd7e8b545fc765fd2babd8dda15175d6f33af1b","status":"completed"}
gh run view 38035583209 --json headSha,status,conclusion
{"conclusion":"","headSha":"4908716b4501312102382e6979b8fc1ded6f9311","status":"in_progress"}
git diff --exit-code 4908716b4501312102382e6979b8fc1ded6f9311 0f3ab280f3d3941b21e2580d584bdaddc614822c -- apps packages deploy tools .github package.json pnpm-lock.yaml
[no output; exit0]
git ls-remote origin refs/heads/codex/hardware-manager-20261010 refs/heads/main refs/heads/codex/hardware-37-20261010 refs/heads/codex/hardware-211-20261010
269d455f6aee29cc89007bbac4aa93d00c0fad7f refs/heads/codex/hardware-211-20261010
0b96a5ff51aca239e2b1492456c37e2052f139ed refs/heads/codex/hardware-37-20261010
0f3ab280f3d3941b21e2580d584bdaddc614822c refs/heads/codex/hardware-manager-20261010
4908716b4501312102382e6979b8fc1ded6f9311 refs/heads/main
```

`git diff --name-only` against the merge shows only six manager task Markdown
documents; no backup or product file was committed. Both manager public checkpoints
correctly distinguish completed PR gate from pending bare-main gate. Exact merged
parents/tree match the tested PR integration. The immutable
[T1 source receipt](https://github.com/mcoder1001-cyber/NGFW/blob/2debe48f2769de473e048544c60261b54f50370c/docs/status/tasks/hardware-37-20261010-test-T1.md)
was read with `git show`: actual hosted checkout8a15d644 and literal
`CI GATE PASSED`,35 Turbo tasks and149 startup checks are pasted there. R7 did not
rerun the full gate or substitute source success for main/physical acceptance.

## Private configuration snapshots

Ready notices were received before inspection. Private contents were never printed
or committed. Inspection used only Python stdlib `os.scandir`/`stat`, JSON manifest
parsing, tar metadata and streaming regular payload lengths, and SHA256 comparisons
against already-public receipt hashes. No configuration values were emitted.

Executed metadata command (Python stdin script; only summary output):

```python
import os, json, stat, tarfile, collections
base = '/root/Documents/Codex/2026-10-10/hardware/recovery-private'
for host, archive, manifests in (
    ('host-37', 'network-config.tar', ('snapshot-manifest.json', 'firewall-fallback-manifest.json')),
    ('host-211', 'network-config.tar.gz', ('capture-status.json',)),
):
    path = os.path.join(base, host)
    counts = collections.Counter()
    def visit(node):
        if isinstance(node, dict):
            for key, value in node.items():
                normalized = key.lower().replace('_', '')
                if ('exit' in normalized or 'returncode' in normalized) and isinstance(value, int):
                    counts[value] += 1
                visit(value)
        elif isinstance(node, list):
            for value in node:
                visit(value)
    entries = list(os.scandir(path))
    for name in manifests:
        visit(json.load(open(os.path.join(path, name))))
    with tarfile.open(os.path.join(path, archive), 'r:*') as tar:
        members = tar.getmembers()
        readable = all(len(tar.extractfile(member).read()) == member.size
                       for member in members if member.isfile())
        management = any(member.name.lstrip('./') == 'etc/netplan/90-ngfw-management.yaml'
                         for member in members)
    print(json.dumps({
        'host': host, 'parent_mode': oct(stat.S_IMODE(os.stat(base).st_mode)),
        'directory_mode': oct(stat.S_IMODE(os.stat(path).st_mode)),
        'regular_files': sum(entry.is_file(follow_symlinks=False) for entry in entries),
        'unsafe_permission_count': sum(bool(stat.S_IMODE(entry.stat(follow_symlinks=False).st_mode) & 0o077)
                                       for entry in entries),
        'manifest_valid_json': True, 'manifest_exit_code_counts': dict(counts),
        'archive_member_count': len(members), 'management_netplan_present': management,
        'all_regular_payloads_readable': readable,
    }, sort_keys=True))
```

The displayed script is formatted for readability; its operations match the
executed `python3 -` command. No private data content is part of the script/output.

Actual metadata inspection output:

```text
host-37: parent_mode=0o700 directory_mode=0o700 regular_files=24
unsafe_permission_count=0 manifest_valid_json=true archive_member_count=5
management_netplan_present=true all_regular_payloads_readable=true
manifest_exit_code_counts={0:10,127:1}
host-211: parent_mode=0o700 directory_mode=0o700 regular_files=26
unsafe_permission_count=0 manifest_valid_json=true archive_member_count=5
management_netplan_present=true all_regular_payloads_readable=true
manifest_exit_code_counts={0:12,127:1}
{"host":"host-37","regular_file_mode_counts":{"0o600":24}}
{"host":"host-211","regular_file_mode_counts":{"0o600":26}}
{"all_match":true,"checked_public_receipt_hashes":5,"matches":5}
```

Numeric exit fields include archive validation, not only remote capture commands;
the .211 gzip validation accounts for its additional zero. Public command receipts
accurately preserve nft exit127. .37 initial8/9 commands plus two fallback captures
succeeded; .21111/12 capture/fallback commands succeeded. Empty successful
iptables-save/ip6tables-save outputs do not prove native nft tables are absent.
The native nft snapshot gap remains explicit before any firewall activation.

All five checked receipt hashes match: both configuration archives, .37 original
and fallback manifests, and .211 final capture manifest. Both five-member selected
archives include the protected management netplan and their regular members are
readable. Scope is selected network configuration and state, not a full system or
data backup. The parent directory initially had0755 permissions; manager corrected
it to0700 before final verification. Nested host directories already protected the
contents; final parent/host/file modes meet the private handling requirement.

Immutable public receipts were fetched, read and their path objects verified:

- [.37 configuration and fallback receipt](https://github.com/mcoder1001-cyber/NGFW/blob/0b96a5ff51aca239e2b1492456c37e2052f139ed/docs/status/tasks/hardware-37-20261010-wip.md).
- [.211 configuration and fallback receipt](https://github.com/mcoder1001-cyber/NGFW/blob/269d455f6aee29cc89007bbac4aa93d00c0fad7f/docs/status/tasks/hardware-211-20261010-recovery-receipt.md).

## Candidate exclusion and recovery boundaries

Read-only Python `tarfile.open(...,'r')` inspected header names in
`/root/Documents/Codex/2026-10-10/hardware/ngfw-hardware-candidate-2045ab8b3d2f.tar`
without extracting or executing files. It compared header path components against
`recovery-private`, and basenames against the actual private directory entries:

```text
{"archive_member_count":23,"private_artifact_basename_matches":0,"private_directory_members":0,"unsafe_paths":0}
```

This applies to the inspected candidate; a later refreshed archive requires the
same exclusion check. Backup bytes remain controller-private and outside Git.

Published recovery runbook at0f3ab280 explicitly requires verified console/rescue,
trusted full off-host backup/image or an agreed data-recovery plan, actual disk
identity and fully unmounted root before diagnostic `e2fsck -fn`, then reviewed
interactive repair without blind `-y`, followed by clean offline check and verified
normal-boot filesystem/management access. Diagnostic errors are not labelled health
success. Config snapshots do not waive storage/data-preservation prerequisites.

Management ports/groups/routes remain explicitly protected. Existing hardware task
branches are retained for resume. Hardware seed/apply/startup steps are proposed,
not live-tested. Installation, binding, firstboot, API/TLS, physical forwarding,
repair and reboot acceptance remain NOT RUN. Operational roles are distinguished
from unchanged board counts and no persistent supervisor is claimed. No R7 overclaim
or new scope/decision-policy violation found in the reviewed public checkpoint.

**Final addendum verdict: APPROVE** the exact public recovery/status checkpoint
`0f3ab280f3d3941b21e2580d584bdaddc614822c` within this evidence scope. Later main-CI
completion/publication must use its actual outcome; hardware acceptance remains
blocked on recovery and is not covered by this approval.

Reviewer's owned-doc checkpoint check: `tools/ci.sh check --base origin/main`
exited0 and printed `check PASSED (0m14s)`. This lightweight check is not the
pending bare-main quick gate or any hardware acceptance test.
