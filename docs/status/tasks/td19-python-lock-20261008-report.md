# TD19 offline lab Python closure candidate generation

The lab installer requires a reviewed complete hash lock but no authoritative release lock is available. The optional builder scripts/lab-python-lock.py makes a candidate reviewable from explicitly pinned direct package inputs and an independently reviewed local wheelhouse. It installs nothing and uses no index. Existing installer defaults remain fail-closed.

Input direct file must contain exactly one name==version entry for pip, robotframework, robotframework-sshlibrary, scapy, pytest and requests. No URLs, directives, markers, floating or duplicate pins. Wheelhouse must contain only bounded regular wheels; symlinks, unsafe ZIP members and direct URL Requires-Dist declarations refuse before pip. A private immutable snapshot feeds trusted current-interpreter pip in isolated mode, configuration disabled, cache disabled, no-index, wheel-only, ignore-installed dry-run. Resolved archive digests must match the snapshot. A second dry-run requires every generated hash and verifies the same full closure.

Run in the intended resolver environment, after reviewing exact direct versions and the upstream authenticity of wheel artifacts:

```bash
python3 -I scripts/lab-python-lock.py \
  --direct /path/to/reviewed-direct.txt \
  --wheelhouse /path/to/reviewed-wheels \
  --output /path/to/new-candidate \
  --target-python 3.14 \
  --target-platform linux-x86_64 \
  --target-os ubuntu:26.04
```

Target flags assert the actual interpreter/environment rather than simulating another runtime. The Python version shown is an explicit intended target input, not a claim that target acceptance occurred. requirements.lock includes complete resolver-selected transitive pins and actual wheel SHA256 values. provenance.json records selected filenames/digests, input/output digests and resolver environment. This receipt proves consistency and offline closure only; it does not authenticate upstream publishers or validate installation/boot. Only independently reviewed release artifacts may become installer inputs.

Synthetic wheels in scripts/tests/lab-python-lock.py are deliberately version 1.0 and never production pins. Nine focused tests passed in 2.634s with zero skips, including actual installer dry-run lock acceptance. Compile/diff/source checks passed. Full hosted quick gate and independent review pending. Reviewed upstream versions/artifacts plus Ubuntu 26.04 target generation/install acceptance remain required; TD19 is not complete.
