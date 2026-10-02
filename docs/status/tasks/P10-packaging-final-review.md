# Independent P10 APT and early-firewall checkpoint review

Frozen reviewer product tree `8b761430`, remote `5453a38149e80cf271678da07561a07bb31ea392`. Scope R1/R2/R7/R8 includes earlier signed APT implementation and new early static firewall. Previous firstboot rulings are preserved; this is a fresh scope review, not full P10 acceptance. Reviewer changed only this report.

## MAJOR findings

1. **Private signing key can enter repository output.** `scripts/publish-apt.sh` excludes source/system output paths but never excludes SIGNING_HOME overlap. If the new output directory is the not-yet-created signing home or an ancestor (e.g. XDG_CONFIG_HOME=/tmp/newroot and output=/tmp/newroot), generating the key creates private material under output, and final chmod -R a+rX makes it publicly readable. Canonicalize both paths upfront and reject equality, output ancestor of signing home, and output descendant of signing home BEFORE any creation/key generation. Add fail-closed path fixtures without needing real signing.
2. **VPP exact dependency validation is only a substring.** The meta Depends predicate accepts `vpp (= correct) | vpp (= wrong)` or a lookalike identifier ending in `vpp`. Thus a successful publication gate does not prove the required unconditional product VPP pin. Parse Debian dependency groups/package identifiers and require exact singleton `vpp (= manifest.version)` without alternatives or conflicting duplicate groups. Extend real dpkg-deb metadata rejection fixtures before key generation/output.

R1/R2/R8 BLOCK pending fixes. No code edits by reviewer. R7 APPROVE checkpoint honesty: dynamic LCP punt synchronization, actual signing/repository install, licensing and capability compatibility remain explicit unfinished work; product release not declared accepted.

## Firewall scope

Pure renderer validates explicit management and bounded unique punt-interface names; no wildcard or control syntax can enter nft source. It creates/replaces only inet vrx_base in one nft batch, keeps host-policy inet vrx separate and does not flush global rules. Early helper requires root, validates bootstrap metadata, checks actual interface presence and nft check before publication, then persists nonsecret interface values for subsequent boots. It has no DB/auth dependency. Main firstboot is ordered after nftables/PG/Valkey; distro nft early ordering is retained, and ExecStop reset avoids destructive global flush. Exact nft kernel syntax/idempotent replay was not run in this environment. Dynamic LCP/punt admission remains a real functional gap, not waived completion.

## Actual independent verification

```text
python3 deploy/debian/vrx/tests/test_base_policy.py
3 tests in 0.002s, OK
python3 deploy/debian/vrx/tests/test_packaging.py
8 tests in 1.476s, OK
python3 deploy/debian/vrx/tests/test_firstboot.py
6 tests in 4.059s, OK
python3 deploy/debian/vrx/tests/test_publish_apt.py
2 tests in 0.265s, OK (skipped=1)
```

APT wrong-pin rejection executed actual dpkg-deb fixture metadata. Real GPG key generation was skipped because isolated gpg-agent cannot start here; signing was NOT RUN, not PASS. No actual reprepro publication was run.

```text
bash -n scripts/publish-apt.sh deploy/debian/vrx/assets/firewall-bootstrap.sh
exit 0
tools/ci.sh check --base origin/main
ok: gitleaks — scanned ~93956 bytes (93.96 KB) in 251ms no leaks found
check PASSED (0m03s)
```

Independent `systemd-analyze verify --root=<private temporary fixture>` with shipped firewall/firstboot units, shipped nft/VPP drop-ins, explicit early nft distro-like ordering and stub dependency targets/services exited 0 with no diagnostics. This proves no cycle in that fixture graph, not actual Ubuntu installed ordering or appliance boot. Real nft unit is absent locally; no host services, netlink writes, package installs or global configuration changes occurred.

Whole P10 remains partial. Full hosted quick and actual build/install/remove/boot acceptance are still required and must not be inferred from offline checks. Original firstboot findings are resolved by separate final rulings, not silently regraded here.
