# Independent PR225 source and evidence review

Same owner-requested hardware installation task; reviewer owns documentation only.
Exact candidate: `4b6cda95669f795116948d6f8b9662d647ba0fef`, parent
`bd25d9b24cb64912f7fdcb76bcf4d5a3c2d7c7b3`. No product or target writes.

Source correctness and management applicability: APPROVE. Initial R7 verdict on
4b6c was BLOCK pending two committed documentation fixes: its task report covered
only firstboot, without the owner-cache failure/correction and actual TestAutoBlock
command/output; the bootstrap options appeared only in the report without the
required decision LOG line. Parent was notified; no new product test was requested.
Both findings are now closed on the exact final candidate below. Mandatory complete
hosted quick remains pending separately.

Actual read-only review commands and selected output:

```text
gh pr view 225 --json headRefName,headRefOid,baseRefName,files,statusCheckRollup,body,title
headRefName=codex/hardware-firstboot-fix-20261010
headRefOid=4b6cda95669f795116948d6f8b9662d647ba0fef baseRefName=main
git fetch origin codex/hardware-firstboot-fix-20261010 codex/hardware-manager-20261010
git diff --stat 4b6cda956^ 4b6cda956
9 files changed, 171 insertions(+), 1 deletion(-)
git diff --check 4b6cda956^ 4b6cda956
# exit0, no output
git rev-parse 4b6cda956^
bd25d9b24cb64912f7fdcb76bcf4d5a3c2d7c7b3
git rev-list --count 4b6cda956^..4b6cda956
1
```

Independent exact-blob comparisons with `git rev-parse <commit>:<path>` prove all
five firstboot product/test paths equal independently tested b1f5bea6 and both
owner-cache paths equal independently tested ee202500. No stale changed product
test claim is carried to this candidate. Actual output:

```text
apps/agent/cmd/ngfw-startupgen/main_test.go reviewed/tested_blob_equal=true 27c038aef199701802be00520eab6f8a122d2899
deploy/debian/ngfw/assets/firstboot.sh reviewed/tested_blob_equal=true cc12b3ca010a9ae0433398ecf381345657828342
deploy/debian/ngfw/assets/initial-dataplane.json reviewed/tested_blob_equal=true a1935dc4e721530fa7ebb9e23f5bbb7c340f2170
deploy/debian/ngfw/debian/ngfw-meta.install reviewed/tested_blob_equal=true fa71c08da9be24ac8df9eeccaf7f900715b1daa9
deploy/debian/ngfw/tests/test_firstboot.py reviewed/tested_blob_equal=true 357c2c59baaad96bc64e0927f3da4c96a5e0a324
apps/agent/internal/agent/rpc_autoblock.go reviewed/tested_blob_equal=true 22a34b1c4555b9525eff9ae5ecdf2278e70585e0
apps/agent/internal/agent/rpc_autoblock_test.go reviewed/tested_blob_equal=true dad16c5afb1bb8bae174ec607fa04fcf28b8a8a1
```

The owner fix validates first, clones the request, then stores the effective
service owner. Caller input stays unchanged. Foreign RPC and persisted ownership
remain rejected, including unchanged-cache proof after a rejected RPC. Existing
loader trust checks are intact. Independent immutable ee202500 archive tests:

```text
# cwd /dev/shm/ngfw-r7-owner-ee202500/apps/agent; owned0700 git archive
env -u NGFW_INTEGRATION go test -mod=readonly -count=1 -run TestAutoBlock -v ./internal/agent
--- PASS: TestAutoBlockOmittedOwnerSurvivesRestart (0.01s)
PASS
ok ngfw/agent/internal/agent 0.350s
# exit0; entire TestAutoBlock selection passed
```

Firstboot source/packaging and real CLI/six private firstboot fixture outputs remain
in [the resumed report](hardware-evidence-review-20261010-plugin-seed.md), immutable
published review6c1b1370d04fd0b0f706be05cde0db5a6df459ba. Shipped input enables exactly
three required plugins, no devices; missing plugin fails closed. Bare-metal setup
documentation correctly separates fresh firstboot from existing completed
appliances. No API/schema/privilege or management binding change is introduced.

Actual remote gate snapshot:

```text
gh run view 38054501474 --json headSha,status,conclusion,jobs
headSha=4b6cda95669f795116948d6f8b9662d647ba0fef
status=in_progress conclusion=""; repository gate step in_progress
gh run view 38054501437 --json headSha,status,conclusion,jobs
headSha=4b6cda95669f795116948d6f8b9662d647ba0fef
status=completed conclusion=success; all fixture steps success
```

Native artifacts/live correction and hardware acceptance remain pending; successful
focused source fixtures do not close required build, hosted gate or target seed.
Next: inspect the amended documentation/head and its unchanged product blobs,
then observe the exact final hosted gate. No duplicate complete quick is run here.

## Final exact candidate

R7 verdict: APPROVE `321581d1850686070afc9ea08621bd6d405dbb08`, tree
`f38fe9c8cbd26ba92f2e3ffb50c700df71658a8d`. The amended task report now includes both
actual failures/fixes, root and independent owner-test commands/output, unchanged
strict foreign-owner/caller-input guards, and pending live artifact/cache/seed/
binding/reboot limits. LOG D-245 records bootstrap options and rationale. No new
schema/privilege decision or out-of-scope work was introduced. Both initial R7
documentation findings are closed.

Exact read-only commands/output:

```text
git fetch origin codex/hardware-firstboot-fix-20261010 codex/archive-hardware-firstboot-ba1a-20261010 codex/hardware-manager-20261010
# exit0
git show --no-patch --format='%H%n%P%n%T' 321581d1850686070afc9ea08621bd6d405dbb08
321581d1850686070afc9ea08621bd6d405dbb08
bd25d9b24cb64912f7fdcb76bcf4d5a3c2d7c7b3
f38fe9c8cbd26ba92f2e3ffb50c700df71658a8d
git diff --exit-code ba1ade118e4fb9d8cf65f7fa085f271789104b54 321581d1850686070afc9ea08621bd6d405dbb08
# exit0, no output: exact approved tree unchanged by D112 consolidation
git diff --check 321581d1850686070afc9ea08621bd6d405dbb08^ 321581d1850686070afc9ea08621bd6d405dbb08
# exit0, no output
git rev-list --count bd25d9b24cb64912f7fdcb76bcf4d5a3c2d7c7b3..321581d1850686070afc9ea08621bd6d405dbb08
1
git ls-remote origin 'refs/heads/codex/archive-hardware-firstboot-*' refs/heads/codex/hardware-firstboot-fix-20261010
4b6cda95669f795116948d6f8b9662d647ba0fef refs/heads/codex/archive-hardware-firstboot-4b6c-20261010
ba1ade118e4fb9d8cf65f7fa085f271789104b54 refs/heads/codex/archive-hardware-firstboot-ba1a-20261010
321581d1850686070afc9ea08621bd6d405dbb08 refs/heads/codex/hardware-firstboot-fix-20261010
gh pr view 225 --json headRefOid,statusCheckRollup,url
headRefOid=321581d1850686070afc9ea08621bd6d405dbb08
Mandatory quick run38055103197 IN_PROGRESS
Offline packaging/signing run38055103328 SUCCESS
```

Final-head complete quick/expected-main merge checks remain required; this approval
does not claim merge readiness while that gate is pending. No repeat complete
quick was run by this reviewer. Native source ee202500 remains distinct from final
integration321581d1: later main also changes gateways.go/pppoe_delegation.go and
tests. Native acceptance must identify its actual source/artifacts precisely,
without claiming byte-identical integration payload. Root was notified.

Root reported actual required build prerequisite failures, retained logs and
unchanged retries: initial umask077 produced DEBIAN0700; normal permissions/owned
TMPDIR all72 VPP fixtures passed. Later private0700 TMPDIR denied an intentional
foreign-UID fixture; normal1777 scratch retry is in progress. No skip/waiver or
successful fixed native archive is inferred. These are root-attributed build
reports, not independent reviewer build execution.

## Mandatory G304 correction and final replacement candidate

The historical pending gate above completed FAILURE on321: mandatory quick
38055103197 found two G304 findings in the new regression's variable-path
os.ReadFile calls. Real gate failure was retained and corrected, without a
linter suppression or test waiver. R7 APPROVE exact replacement
`3b61a8ce529c68ae2bb39e2cea2f77ea13602595`, tree
`507477988ae381f05e1e8dc823d8479590b1efa1`, single parentbd25d9b24.

Independent fetched diff321→3b61 contains only the regression's fixed-leaf
`fs.ReadFile(os.DirFS(ownTempDir), "auto-block.json")` reads and truthful task
report of failed quick/focused correction outputs. Production is unchanged.
Root-attributed corrected TestAutoBlock output0.312s and golangci-lint agent
selection0issues appear in the report; this reviewer did not duplicate those
tests. Reviewed321 history was preserved in the exact remote archive first.

```text
git show --no-patch --format='%H%n%P%n%T' 3b61a8ce529c68ae2bb39e2cea2f77ea13602595
3b61a8ce529c68ae2bb39e2cea2f77ea13602595
bd25d9b24cb64912f7fdcb76bcf4d5a3c2d7c7b3
507477988ae381f05e1e8dc823d8479590b1efa1
git diff --stat 321581d1850686070afc9ea08621bd6d405dbb08 3b61a8ce529c68ae2bb39e2cea2f77ea13602595
2 files changed,24 insertions(+),2 deletions(-)
git ls-remote origin refs/heads/codex/archive-hardware-firstboot-3215-20261010
321581d1850686070afc9ea08621bd6d405dbb08 refs/heads/codex/archive-hardware-firstboot-3215-20261010
gh pr view 225 --json headRefOid,state,mergeStateStatus,statusCheckRollup,url
headRefOid=3b61a8ce529c68ae2bb39e2cea2f77ea13602595 state=OPEN mergeStateStatus=UNSTABLE
Mandatory quick38056374923 IN_PROGRESS
Offline packaging/signing38056374912 SUCCESS
```

Fresh complete quick and expected-head/main checks remain required before merge.
New native artifacts completed on distinctee202 source, as precisely reviewed in
[the four-package report](hardware-evidence-review-20261010-upgrade-four.md).
The test-only correction does not relabel their producer source or claim current
target seed/physical NIC/forwarding/reboot acceptance.

## Observed green gate and exact merge

```text
gh run view 38056374923 --json headSha,status,conclusion
headSha=3b61a8ce529c68ae2bb39e2cea2f77ea13602595 status=completed conclusion=success
gh pr view 225 --json state,mergeCommit,mergedAt,headRefOid,url
state=MERGED headRefOid=3b61a8ce529c68ae2bb39e2cea2f77ea13602595
mergeCommit=d1f3f19d4837de3f7a36bfffcbd3c70593bea307
gh api repos/mcoder1001-cyber/NGFW/git/commits/d1f3f19d4837de3f7a36bfffcbd3c70593bea307 --jq '{sha:.sha,tree:.tree.sha,parents:[.parents[].sha]}'
parents=bd25d9b24cb64912f7fdcb76bcf4d5a3c2d7c7b3,3b61a8ce529c68ae2bb39e2cea2f77ea13602595
tree=507477988ae381f05e1e8dc823d8479590b1efa1 # exact approved/tested candidate
gh run view 38057527122 --json headSha,status,conclusion
headSha=d1f3f19d4837de3f7a36bfffcbd3c70593bea307 status=in_progress conclusion=""
```

Actual final-head gate and merge PASS; postmerge main gate remains pending
separately. Native sourceee202 actual upgrade/cache/runtime/seed17 now PASS in
the four-package report; physical NIC/forwarding/reboot acceptance is still open.
