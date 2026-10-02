# TD-19 repository key gate — independent R1/R2/R5 review

Frozen `53f699393362b5773ba89c00b506c4fc2a141d6e`, isolated task/TD19-repo-key-review. Scope is operator-supplied exact primary key pins and bounded source/parser fixtures; no real key retrieval, crypto verification, APT or host mutations.

## MAJOR — invalid/disabled/unknown primary validity accepted

`scripts/00-add-repos.sh:89–91`: pub validity check rejects only `r` and `e`. Controlled GPG output carrying exact trusted primary fingerprint with validity `i`, `d` or `?` is accepted and dearmored. This contradicts the declared invalid-primary refusal and fail-closed parser contract. Minimal pub field length is only2, so malformed identity structure is also insufficiently checked.

Personally invoked existing fixture's actual extracted verify_repo_key function with each validity variant; all three produced exit0 and temporary output. This is a real parser acceptance gap, not real cryptographic key acceptance proof. Exact fingerprints still prevent substitution of a different primary identity; no such trust bypass is claimed.

Fix: explicitly reject invalid/disabled/unknown primary statuses and malformed supported pub records, retain ordinary untrusted-but-valid public status needed in a private empty GPG home, and add executable invalid/disabled/unknown structure fixtures. Review expiration metadata/capability semantics against GPG output contract so a valid fingerprint does not imply usable key. Do not replace trusted pins with fingerprints extracted from downloads.

## Other source findings / boundaries

- Missing/malformed/duplicate/oversized trusted fingerprint sets refuse before artifact verification/network/APT. Both key sets are validated before global keyring writes; exact primary set equality rejects extra/missing/duplicate identities. No authoritative default identity was invented: administrator trust remains explicit and unresolved for automatic unattended bootstrap.
- Downloads use fixed HTTPS URLs, maximum1MiB files under mktemp private work directory; lstat regular/nonempty size bounds precede GPG. Production files are in the private root work directory, so ordinary unprivileged path substitution between lstat and GPG is prevented by that ownership boundary. Helper arbitrary-path fixture use does not authorize caller-controlled production paths.
- Private0700 GPG homes and --no-options/batch prevent user config/global keyring access. Secret packet/identity rejection occurs before dearmor. Subkey/UID data are not accepted as primary fingerprints. This review does not assert controlled stubs prove real key signature validation or authoritative repository ownership.
- Global output uses0644 sibling mktemp/install followed by rename-T; target final symlink is replaced rather than followed. Root-owned keyring parent trust is required. Keyring replacement is per file, not an all-files transaction: a later rename/APT/source-write failure can leave a previously validated key installed; do not claim cross-file rollback. No downloaded shell/input is sourced and no secret is printed.
- MINOR: new curl downloads have a size bound but no total/connect time deadline; add bounded retry/time limits for manager liveness when convenient. This is not a new trust bypass.

## Personally executed

```
python3 docs/status/tasks/TD-19-run-fixtures.py
Ran30 tests in5.935s — OK
fixtures: failures0 errors0 skipped0
```

Seven key fixtures exercise controlled GPG argument/parser output; they are not real cryptographic tests. Additional invalid/disabled/unknown status reproduction: each exit0/outputTrue (unexpected). All work stayed in temporary fixture paths, with network/GPG substituted and no global installation.

**Verdict: BLOCK (1 MAJOR primary validity/parser gap). Full TD19 remains unfinished; authoritative pins and real source/host acceptance are not resolved by these fixtures.**
