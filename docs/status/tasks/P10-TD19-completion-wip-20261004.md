# Packaging and provisioning completion checkpoint

Branch: `codex/packaging-complete-running-20261004`; local base and remote base
`4c8d1b247`. No publication of this checkpoint asserted before push succeeds.
Owned: `TD-19-test-shell-source.py`, completion envelope and this report.

Completed correction: existing TD-19 teardown fixture omitted the subsequently
integrated classify-reset helper and failed with exit127. The harmless stub now
records that helper's exact interface argument and the assertion checks reset
occurs before delete, preserving all prior success/failure fallback cases.
No production behavior changed or fixture assertion disabled.

Actual checks on current main source: Debian packaging **30/30 PASS** (40.677s),
FRR certificate selector **13/13 PASS** (21.257s), Go module reader **6/6 PASS**
(0.926s). TD-19 strict runner before fix: 36 tests, one failure described above;
focused corrected shell fixture **4/4 PASS** (0.157s). Full strict rerun pending
at `/tmp/ngfw-td19-completion-fixtures-20261004.log`. `git diff --check` PASS.
Full CI not run here; manager retains the applicable owner gate authorization.

Actual artifact attempt: unchanged `deploy/vpp/build.sh --offline-reference
--jobs 2` cloned `/root/vpp` read-only into this worker's ignored `.build`,
verified pinned tag and commit, applied existing product patches, obtained
`26.06-release+ngfw3`, and verified all46 existing build dependencies. It then
failed because pinned `meson-0.57.2.tar.gz` wheelhouse input was absent. No .deb
was produced. Online build retry running, session47043; log
`/tmp/ngfw-packaging-vpp-build-online-20261004.log`.

Fresh read-only official authority research, 2026-10-04: [FRR official
repository](https://deb.frrouting.org/) still lists only the original three
trusted fingerprints, while prior independent InRelease examination needs the
fourth signer. No new authorization of that signer located. [NodeSource
DEV_README](https://github.com/nodesource/distributions/blob/master/DEV_README.md)
still ends Ubuntu support at24.04. No independent official default fingerprint
authority located. This is bounded negative evidence; downloaded keys do not
authenticate their own ownership. Current explicit operator trust remains.

Remaining code/decisions: authoritative source copyright/license missing;
CAP_CHOWN and atomic global `/etc` writer decision pending; verified product VPP
artifacts absent; complete portable dependency closure and actual signed
runtime bundle unbuilt. These are not deferred laboratory acceptance. Fresh
target install/boot/traffic tests remain NOT RUN. Dynamic punt admission is
already integrated; historic report saying it is unbuilt is superseded.

Exact next command: `tail -30 /tmp/ngfw-packaging-vpp-build-online-20261004.log`;
then inspect strict fixture log and publish this coherent correction.

Follow-up: corrected strict TD-19 runner **36/36 PASS** in69.065s, zero skipped
or expected failures. Local source checkpoint `d3a3db82b`; CLI push attempted
and rejected with HTTP403, so publication is not established. Manager notified
to use the authorized GitHub connector. Online VPP build passed pinned wheel
and external source validation and entered isolated source compilation.
