# Independent PR225 source and evidence review

Same owner-requested hardware installation task; reviewer owns documentation only.
Exact candidate: `4b6cda95669f795116948d6f8b9662d647ba0fef`, parent
`bd25d9b24cb64912f7fdcb76bcf4d5a3c2d7c7b3`. No product or target writes.

Source correctness and management applicability: APPROVE. Final R7 verdict:
BLOCK pending two committed documentation fixes. The task report currently covers
only the firstboot correction; add the actual owner-cache failure, correction and
TestAutoBlock command/output for the second product change. Add the required
decision LOG line for explicit bootstrap input versus global renderer injection
or disabled policy; the options currently appear only in the task report. No new
product test is requested for these documentation changes. Mandatory complete
hosted quick remains pending separately. Parent notified of these exact findings.

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
