# RA helper build compaction contract

Branch: codex/ra-helper-compaction-20261006. Source base local 7af31dfb78c1681ca40c6e9929caf910cbb0cb0e / remote 0941ad72641e1ee03dd9ee07e2ecb2c7e3add41a / tree d880451a82409684b59854781ea0806211f8e587.

Owned product files: deploy/debian/ngfw/prepare.sh and tests/test_prepare.py only. Owned documentation: this contract and F-ra-helper-compaction-{wip,envelope,report}.md.

Add -ldflags="-s -w" only to the two RA helper Go builds before unchanged full SHA manifest generation. Preserve CGO_ENABLED=0, trimpath, other builds, units, Debian strip exclusions, all runtime identity/hash/size/budget guards. Fresh artifacts have fresh identity, never old SHA equivalence.

Authorized validation: scoped packaging tests, unchanged meaningful proof negatives, compile only public helpers into fresh owned artifact paths with heavy semaphore/GOMAXPROCS=2/-p=2; inspect ELF/buildinfo/digests without executing or installing helpers. No guest/service/private inputs or operational Ready/performance claim. Independent review required before manager integration.
