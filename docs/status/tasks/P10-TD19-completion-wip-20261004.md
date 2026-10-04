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

Connector publication subsequently succeeded. Remote checkpoint
`f367c442fbc6497681e636892eb95daeece99b73`, tree
`1894a39894ffcce56fa5de6599fdcc0e756f08c6`, equals local `b0c411dcb` tree,
including fixture correction, independent review and exact trust-material report.
The original active session ended during worker handoff; detached nohup retry
also had no persistent-process evidence. Actual build restarted in live session
56026, log `/tmp/ngfw-packaging-vpp-build-active-20261004.log`. Do not call this
a persistent runner. Cached wheel/source inputs verified; product version remains
26.06-release+ngfw3. No completed runtime Debian manifest exists yet.

Actual isolated static Go builds for agent/startupgen/vppcheck all succeeded;
inputs are this source checkpoint, output `.scratch/product-build/bin/`.
Own frozen pnpm installation PASS5.4s; API/web build in progress, log
`/tmp/ngfw-packaging-app-build-20261004.log`. Authoritative license and privilege
questions remain pending; no product release readiness is claimed.

API/web build finished:14/14 Turbo tasks PASS,7 cached,1m0.627s. Generator
created two pre-existing stale YANG differences on baseline4c8; those generated
paths were restored rather than published as a clean release. Bounded source
correction: prepare.sh now rebuilds API/web dependencies before staging, refuses
build failure or tracked generation drift before creating output, and cannot
mislabel old ignored dist outputs with the current source SHA. Three actual
driver fixtures PASS1.393s: stale output refreshed; build failure refused;
tracked mutation refused. Shell syntax, shellcheck and whitespace PASS.
Independent manager review still required. No privilege or licensing deviation.
