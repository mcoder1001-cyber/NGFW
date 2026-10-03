# P10 offline installation work checkpoint

Branch `codex/p10-offline-install-20261003`, base `a044cf1d`; manager owns authenticated remote publication. No remote publication claimed by worker.

Initial implementation: mandatory external expected manifest; bounded no-follow descriptor snapshot; existing VPP/full bundle verification; read-only default; explicit target/root guard; fixed apt-get simulation then no-download/no-remove install using private isolated sources/cache/config and minimal environment. No caller shell or package/service operation executed during development.

Actual validation so far: `python3 -m py_compile deploy/debian/bundle/install.py` PASS. Fixture tests, independent review and unchanged full CI still pending. No real complete bundle, chroot installation or hardware acceptance exists.

Next command: `python3 -m unittest discover -s deploy/debian/bundle -p 'test_install.py' -v` after writing bounded synthetic dpkg fixtures. Remaining: race/security tests, fixture workflow step, accurate user guide, independent review, hosted gate, real release artifacts and target acceptance. APT may execute trusted privileged maintainer scripts; no-download is not a network sandbox for those scripts.
