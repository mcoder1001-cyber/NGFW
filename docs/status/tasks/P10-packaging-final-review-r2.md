# Independent P10 APT fix verification, round 2

Reviewed frozen local `a34e0d6e792fd95f8ad1802249d0423ff38a7bf3`, remote `3d13f8fd4fed7a07ca9d2d9f7231d669a0e09e4c`. Original BLOCK at `8b761430` preserved. Scope is bounded verification of the two APT findings, not full release acceptance. Reviewer changed only this report.

Both original MAJOR findings resolved:

- Signing-home and output are canonicalized with realpath -m before any creation. Both directional ancestry tests cover equality and parent/child relationships including nonexistent suffixes; signing keys cannot be created under publication output or publication inside signing home through the documented paths. Independent existing-symlink-parent fixture also rejected overlap before mutation. Trusted builder ownership remains required; script is not protection against a concurrent malicious filesystem actor.
- Debian dependency validation identifies VPP package groups, requires exactly one, and uses full equality matching against escaped verified version. Alternatives, duplicates, prefix-lookalike-only metadata and non-equality relations reject before repository/signing creation. Colon-qualified/alternative VPP entries also cannot satisfy the full exact match. Manifest install verification remains in place.

R1/R2/R7/R8: **APPROVE reviewed APT/firewall checkpoint scope** after bounded verification. No unresolved MAJOR in these fixes. This approval does not establish real GPG/reprepro publication, actual appliance packaging/boot or whole P10 completion. Existing dynamic punt admission, capability compatibility, licensing and release gates remain explicit.

Actual independent checks:

```text
python3 deploy/debian/ngfw/tests/test_publish_apt.py
Ran 4 tests in 1.341s
OK (skipped=1)
```

Three actual rejection test methods passed using real dpkg-deb fixture metadata. Real signing method skipped because isolated gpg-agent cannot start in this environment; signature validation NOT RUN, not PASS.

Additional independent fixture: XDG_CONFIG_HOME through an existing directory symlink with a nonexistent child, publication output through canonical ancestor of the future signing home: `symlink ancestor rejected: True; no mutation: True`.

```text
bash -n scripts/publish-apt.sh
exit 0
tools/ci.sh check --base origin/main
ok: gitleaks — scanned ~97775 bytes (97.78 KB) in 173ms no leaks found
check PASSED (0m02s)
```

No host service starts, firewall/netlink writes, live publication or full quick PASS asserted. Final hosted quick on the exact integration checkpoint remains required before merge.
