# WIP: deploy-host-37-20261019 — upgrade 172.30.126.37 to main

Branch: `claude/machine-172-30-126-37-update-c90ed0` (cut from `origin/main` `cb148aafb`).
Owner request (2026-10-19): "ماشین 172.30.126.37 را بروزرسانی کن" — bring the appliance
`root@172.30.126.37` to the current integrated `main` product.
Owned files: `docs/status/tasks/deploy-host-37-20261019-wip.md` only; no product code changed.

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
- Result: pending.

## Upgrade

- Command: `deploy/deploy-hosts.sh -p /var/tmp/ngfw-build-main-cb148aafb-20261019/packages 172.30.126.37`.
- Lineage: installed `8eb2f11c4069` is an ancestor of `cb148aafb`; no `--allow-divergent` needed.
- Result: pending.

## Verification after the upgrade

- Pending.

## Exact next command

`deploy/deploy-hosts.sh -p /var/tmp/ngfw-build-main-cb148aafb-20261019/packages 172.30.126.37`
