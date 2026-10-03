# Independent T1 P11 fixture evidence

Tester /root, independent of developer.
Worktree work/NGFW-p11-resume.
Final tested2dbdff2405e845da47aa149bb72cdedbce60649c.
Tree406ff6de90fb16708d6523816eb5003f5f5b93b6.

Actual commands/output:
```text
git rev-parse HEAD
2dbdff2405e845da47aa149bb72cdedbce60649c
git rev-parse HEAD^{tree}
406ff6de90fb16708d6523816eb5003f5f5b93b6
PYTHONDONTWRITEBYTECODE=1 python3 deploy/strongswan/test_verify_inputs.py
9 named cases each ... ok
----------------------------------------------------------------------
Ran9tests in35.523s
OK
git diff --check
(no output, exit0)
```

Cases: valid metadata/report invariance; invalid digest/version/metadata; missing, duplicate or unsafe staging entries; missing generated release inputs; compressed/decompressed bounds; symlink and replacement refusal; unsafe tar paths/link/special files; truncated bzip CLI refusal; real VPP gate refusal of the synthetic build. The positive provenance boundary is explicitly stubbed. No real tar compatibility, authenticated upstream release, plugin compilation, package installation or ESP traffic was verified.

Earlier9fixtures on27d3d800 passed18.056s but lacked the missing-digest negative case; independent R2 inspection found it and required correction. The corrected suite covers that case inside test_hash_version_and_metadata_mismatch.

Focused verdictPASS.
Overall T1 integration PENDING exact unchanged hosted complete quick and mandatory fresh panel.
