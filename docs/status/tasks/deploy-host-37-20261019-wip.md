# WIP: deploy-host-37-20261019 — upgrade 172.30.126.37 to main

Branch: `claude/machine-172-30-126-37-update-c90ed0` (cut from `origin/main` `cb148aafb`).
PR: https://github.com/mcoder1001-cyber/NGFW/pull/244 (open; quick gate and independent review pending).
Owner request (2026-10-19): "ماشین 172.30.126.37 را بروزرسانی کن" — bring the appliance
`root@172.30.126.37` to the current integrated `main` product.
Owned files: `docs/status/tasks/deploy-host-37-20261019-wip.md`, `deploy/deploy-hosts.sh`
(one APT-argument fix, below), `docs/install/deploy-hosts.md` (matching paragraph).

## Target state before the upgrade (read-only, 2026-10-19 07:28 UTC)

- Installed: `ngfw-agent`/`ngfw-api`/`ngfw-meta`/`ngfw-web` `0.1.0~dev+8eb2f11c4069`
  (legacy hash-only version; source `8eb2f11c4` "contract(setup): optional WAN and reliable
  appliance first boot", an ancestor of `origin/main`, 8 commits behind).
- VPP runtime `26.06-release+ngfw3`; agent/API/nginx/VPP active since 2026-10-14 15:10 UTC;
  `NRestarts` 0/0; `nginx -t` ok; HTTPS `/` 200; API health ok; Python 3.14.4; 38 GiB free.
- Management route `default via 172.30.126.1 dev enp12s0`; `/etc/vpp/startup.conf` SHA256
  `585fedd51cf19d91ca0708a6a6387a034a0b0076f5d6e12c367b57983802e7dc`.
- Only failed unit: `systemd-networkd-wait-online.service` (pre-existing since 2026-10-10).
- Local deviations found: `/usr/bin/ngfw` (reports `4f70b3315b96`) and the served web bundle
  (`/usr/share/ngfw/web/index.html` → `index-BB7DUMZW.js`, copied 2026-10-14 14:53) are not
  owned by any package: a hand-applied overlay. The package upgrade replaces `index.html` and
  `/usr/bin/ngfw` with the built files; stale unowned asset files stay on disk (harmless).

## Build

- Source: `origin/main` `cb148aafb` (full history, count 695). Verified VPP output:
  `/root/Documents/Codex/2026-10-04/debian-latest/outputs/NGFW-Debian-latest-development/VPP-26.06-release+ngfw3-amd64`
  (manifest SHA256 `aeea8136…1054`; `deploy/vpp/verify.sh --require-files … --install-gate` → OK,
  `tests/run.sh: 72 passed, 0 failed` when run on its own).
- Command: `NGFW_VPP_IN_TESTS=1 deploy/deploy-hosts.sh -b -r main -v <VPP> -w /var/tmp/ngfw-build-main-cb148aafb-20261019`.
- Deviation: the first run without `NGFW_VPP_IN_TESTS` failed in the VPP gate: `deploy-hosts.sh`
  sets `umask 077` (since `99a774734`) and `deploy/vpp/tests/run.sh`'s `mkdeb` fixture then builds
  control directories with mode 0700, which `dpkg-deb --build` refuses
  ("control directory has bad permissions 700"), so tests 61/62/69 report
  "package set != VERSION VPP_PACKAGES". Reproduced with `umask 077` alone; `LC_ALL=C` is not
  involved. The variable only skips `tests/run.sh` (already passed standalone); every output
  check and the install gate still ran. Follow-up: make the fixture umask-safe (test-only fix).
- Result: `0.1.0~dev+git695.cb148aafb288` (agent/api amd64, meta/web all), `build-info.json`
  `{source_commit cb148aafb288…1698, source_count 695, vpp_manifest_sha256 aeea8136…1054}`,
  packaging tests in `dh_auto_test` all OK, every packaged file group/other-readable (dh_fixperms),
  `deploy-hosts.sh -b -p <packages>` validation OK. Work directory
  `/var/tmp/ngfw-build-main-cb148aafb-20261019` (retained; packages in `packages/`).

## Upgrade

- Command: `deploy/deploy-hosts.sh -p /var/tmp/ngfw-build-main-cb148aafb-20261019/packages 172.30.126.37`.
- Lineage: installed `8eb2f11c4069` is an ancestor of `cb148aafb`; no `--allow-divergent` needed.
- Run 1 (07:46 UTC, script as on `main`): preflight and lineage passed, packages copied,
  backups taken in `/root/ngfw-deploy/run.c65TGMK8`, APT simulation accepted (only the four
  packages), then the real APT call failed before dpkg ran:
  `E: Internal Error, Pathname to install is not absolute 'ngfw-meta_0.1.0~dev+git695.cb148aafb288_all.deb'`.
  Host unchanged (versions, services, dpkg log). Cause, reproduced in an isolated throwaway APT
  state on 195 with `Debug::pkgDPkgPM` (dpkg never executed): apt 3.2.0 refuses
  `--no-download install ./x.deb` with or without absolute paths; the `file:` fetch that turns the
  local archive into the absolute path handed to dpkg never runs under `--no-download`. The flag
  entered the script in `99a774734` (2026-10-15) and had only been exercised by the fake-APT
  fixtures. Fix (`c48ea7e19`): hide every repository for both APT calls
  (`-o Dir::Etc::SourceList=/dev/null -o Dir::Etc::SourceParts=/dev/null`) instead; verified in the
  same isolated state that a local archive installs, a dependency that would need a download is
  refused ("not installable"), `--simulate` still works and `apt-get --print-uris update` lists
  zero sources. Run 1's recovery directory is left in place (backups, `apt-plan.txt`,
  `service-status.txt`).
- Run 2 (07:56 UTC, fixed script): preflight OK, lineage `8eb2f11c4069` ancestor of
  `cb148aafb288`, recovery directory `/root/ngfw-deploy/run.NqsbuRab` with
  `preupgrade-config.tar.gz`, `preupgrade-ngfw.dump`, `previous-versions.txt`, `apt-plan.txt`,
  `default-route-v{4,6}.txt`, `nginx-test.txt`; APT `Need to get 0 B/48.7 MB` (no downloads),
  four packages upgraded, agent/API restarted, nginx reloaded, API health OK, 10 s stability
  window passed, routes unchanged, `upgrade-complete` written. Script exit 0 at 07:57:04.

## Verification after the upgrade

Read-only checks over SSH at 07:57–08:00 UTC:

- `ngfw-agent`/`ngfw-api`/`ngfw-meta`/`ngfw-web` `0.1.0~dev+git695.cb148aafb288`, `vpp`
  `26.06-release+ngfw3`; `dpkg --audit` empty; `dpkg -V` reports only the usual missing
  `/usr/share/doc` changelogs.
- agent/API/nginx/VPP active, `NRestarts` 0 for all; nginx not restarted (active since
  2026-10-10, reload only); `nginx -t` OK; HTTPS `/` 200 serving the new bundle
  `assets/index-BnO7SXiT.js`; served `index.html` MD5 equals the package's; API health
  `{"status":"ok"}`; `/usr/bin/ngfw --version` → `ngfw cb148aafb288`, now owned by `ngfw-api`.
- Management route unchanged; `/etc/vpp/startup.conf` SHA256 unchanged
  (`585fedd5…02dc`); no agent/API/nginx journal errors since the upgrade; the only failed unit
  is still the pre-existing `systemd-networkd-wait-online.service`.
- **VPP was restarted** (07:56:48 SIGTERM → 07:56:58 started; ≈9 s data-plane gap), although
  the script itself only restarts agent/API. Journal order: dpkg's `ngfw-meta.prerm` runs
  `deb-systemd-invoke stop ngfw-firewall-bootstrap.service ngfw-firstboot.service`; on this host
  `vpp.service` carries `Requires=ngfw-firstboot.service` (plus the hardware-37 bind and
  dataplane-performance drop-ins), so systemd stopped VPP and, through `Requires=vpp.service`,
  the agent; the script's restart brought everything back. VPP is up with the same 7 physical
  interfaces up (`enp10s0`, `enp11s0`, `enp13s0`–`enp17s0`) and the agent reconciled its dynamic
  sources. The same VPP restart was recorded for the 2026-10-13 upgrade. Follow-up (product):
  stop `ngfw-meta` from stopping provisioning oneshots on upgrade (`--no-stop-on-upgrade`) or
  drop the `Requires=` coupling; until then every package upgrade on such hosts is a short
  data-plane outage and needs a maintenance window.
- Not run: login/browser acceptance, dataplane forwarding tests, backup restore.

## Host-independent checks on this branch (`c48ea7e19`)

- `bash -n deploy/deploy-hosts.sh` OK; `shellcheck --external-sources deploy/deploy-hosts.sh` clean.
- `python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_deploy_hosts.py -v`: Ran 33 tests in 654.288s — OK (exit 0)
- Not run here: the hosted quick gate (required before merge).

## Follow-ups

1. Merge the APT fix (PR from this branch) after independent review and the quick gate.
2. Test-only fix: make `deploy/vpp/tests/run.sh`'s `mkdeb` umask-safe so `deploy-hosts.sh -b`
   no longer needs `NGFW_VPP_IN_TESTS=1`.
3. Packaging decision: VPP restart on every `ngfw-meta` upgrade (above).

## Exact next command

None for the upgrade (complete). For the branch: read PR #244 checks (`gh pr checks 244`), fix real failures, obtain an independent review, then merge sequentially.
