# P10 offline installation work checkpoint

Branch `codex/p10-offline-install-20261003`, base `a044cf1d`; manager owns authenticated remote publication. Initial checkpoint `dc6576f8` published by manager as remote `codex/p10-offline-install-checkpoint-20261003` SHA `a49a1645d90e4c94f328bb8cfd29b4564a146d2b`, source tree `cceed4ba2029ee5b406fcb9696b88672ce930a6f`; this is not a final reviewed delivery.

Initial implementation: mandatory external expected manifest; bounded no-follow descriptor snapshot; existing VPP/full bundle verification; read-only default; explicit target/root guard; fixed apt-get simulation then no-download/no-remove install using private isolated sources/cache/config and minimal environment. No caller shell or package/service operation executed during development.

Actual validation so far: `python3 -m py_compile deploy/debian/bundle/install.py` PASS. Fixture tests, independent review and unchanged full CI still pending. No real complete bundle, chroot installation or hardware acceptance exists.

Next command: `python3 -m unittest discover -s deploy/debian/bundle -p 'test_install.py' -v` after writing bounded synthetic dpkg fixtures. Remaining: race/security tests, fixture workflow step, accurate user guide, independent review, hosted gate, real release artifacts and target acceptance. APT may execute trusted privileged maintainer scripts; no-download is not a network sandbox for those scripts.


Coherent fixture/doc increment: nine installer tests PASS in 6.775s, zero skips;
`tools/ci.sh check --base a044cf1d` initial PASS with local missing-gitleaks WARN,
not the complete merge gate. No APT package/service/network operation executed.
Added only the installer fixture step to provisioning workflow; existing suites
unchanged. User guide now explains explicit operation, partial-failure risk,
maintainer-script privilege/network limitations and remaining real acceptance.
Next: rerun checker/installer fixtures and branch check, commit, independent
R1/R2/R7/R8 review and hosted unchanged quick gate via manager. Real P10 remains
incomplete; this branch adds only bounded explicit installation orchestration.
