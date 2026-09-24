# Task P10 — Debian packaging + systemd + install script   (prepend 00-CONTEXT.md)

## Goal
Turn the three apps into signed `.deb` packages that install on a clean **Ubuntu 26.04** and
come up as a working appliance, driven by systemd, with the correct privilege split. VPP comes from our
source-built packages (D-001), never from the FD.io repo: since F-vpp-debs merged that means the output of
`deploy/vpp/build.sh` — only packages with `ship: true` in its `manifest.json` (the 7 runtime packages; never `vpp-dbg`,
`vpp-dev`, `libvppinfra-dev`, `vpp-plugin-devtools` — D-089), version `26.06-release+vrx<N>` (D-092), consumed only after
`deploy/vpp/verify.sh --require-files <out>` passes. The earlier build output was lost with the F-vpp-debs worktree (D-098)
and `/srv/vrx-artifacts/` does not exist yet: rebuild into your worktree's `deploy/vpp/.build/` (git-ignored; ≈45 min,
`nohup`). The locked wheelhouse (`.build/pydeps/wheelhouse`) and the external tarball cache were lost too, so even
`--offline-reference` needs PyPI + the DPDK tarball once — if they are unreachable, write it in the questions file (PENDING-network)
and build the rest against a placeholder `Depends:`. Hand the output directory to the manager, who publishes
it to `/srv/vrx-artifacts/vpp/<version>/` (you decide the APT repository layout). `/root/vpp/build-root/*.deb` (the host's own
`26.06-release` build) may be read for a chroot smoke test only — it is never what the product ships.

## Read first
`docs/09-os-packages.md`, `scripts/10-install-runtime.sh`, `docs/01-architecture.md` AD-6, `deploy/vpp/README.md` (the P10 bullets),
`docs/agent/renderers/vppstartup.md` (the `vrx-startupgen` CLI), `apps/api/src/config.ts` (the API's `VRX_*` environment),
`docs/decisions/LOG.md` D-028, D-057, D-071, D-079, D-081, D-084, D-086, D-089, D-092, D-100, D-103.

## Build exactly this
1. Packages: `vrx-agent` (static Go binary, root, `After=vpp.service`, `Requires=vpp.service`,
   `Restart=always`, creates `/run/vrx` and `/var/lib/vrx/agent`), `vrx-api` (bundled Node app
   via `pnpm deploy` + system `nodejs` 22 dependency, user `vrx`, no capabilities,
   `ProtectSystem=strict`, `NoNewPrivileges=yes`), `vrx-web` (built SPA + nginx site config +
   TLS bootstrap with a self-signed cert on first boot; nginx never proxies `/api` from plain :80 — redirect only (D-100);
   443 proxies `/api` incl. the WebSocket `/api/v1/stream`, `/restconf` and `/.well-known/host-meta` to 127.0.0.1:3000),
   `vrx-meta` (depends on all + `vpp (= <manifest.version>)` (ours), `frr` (deb.frrouting.org frr-stable, host 10.7.1),
   `postgresql (>= 16)` (26.04 ships 18, D-028), `valkey-server`, nginx, nodejs 22 (nodesource), and the daemons the renderers drive:
   kea-dhcp4-server + kea-dhcp6-server (never kea-ctrl-agent, D-079), unbound, chrony, rsyslog + its TLS driver (D-086), snmpd,
   keepalived, nftables; strongSwan is `vrx-strongswan` from P11 (its `deploy/debian/vrx-strongswan/**` is P11's — never create or edit it;
   `Recommends:` it if P11 has not merged); hsflowd is not in the archive → out, listed. `vrx-meta` ships the
   `vrx-firstboot.service` that creates the DB, runs the Drizzle migrations (`apps/api/migrations`), generates `VRX_JWT_SECRET` and the
   `VRX_SECRET_KEY_FILE` (0600), seeds the admin from `/etc/vrx/bootstrap.env` (`VRX_BOOTSTRAP_ADMIN_USER/PASSWORD`, file deleted after use),
   renders `/etc/vpp/startup.conf` with **`vrx-startupgen`** (F-startup-gen; built from `apps/agent/cmd/vrx-startupgen`, shipped in
   vrx-agent) from the default `dataplane` document — no devices → `dpdk { no-pci }` — then disables itself. No hand-written
   `startup.conf.tmpl`: later startup.conf changes go through `deploy/vpp/apply-startup.sh` (F-startup-apply + TD-6, handover gate), which
   vrx-agent ships. The product agent runs with `VRX_GLOBALS_OWNER=1` (D-071; the shared host's `tools/app` uses 0).
2. `deploy/debian/vrx/` (one source package building vrx-agent, vrx-api, vrx-web, vrx-meta) with proper `debian/control`, `rules` (dh),
   `postinst/prerm` that are idempotent and never destroy data on upgrade; versions from git tags (`0.1.0~dev+<sha>`).
3. `deploy/apt/`: `reprepro` config + `Release` signing with a GPG key generated locally into `~/.config/ngfw/apt-signing/` (never committed,
   never printed); `scripts/publish-apt.sh`; include our VPP debs in the repo. The repo itself is build output (outside git): the manager
   publishes it to `/srv/vrx-artifacts/apt/` for P14, F-images and F-hardening-lite.
4. `deploy/systemd/vrx-*` hardening (`deploy/systemd/ngfw-manager.service` is the manager's supervisor — never touch it): agent unit gets
   `CapabilityBoundingSet=CAP_NET_ADMIN CAP_SYS_ADMIN CAP_IPC_LOCK` and nothing more; api unit gets none; both `PrivateTmp`, `ProtectHome`;
   api `RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6`; **agent additionally `AF_NETLINK`** (rtnetlink: TD-5's af_packet quiesce, the
   nftables renderer, linux-cp — without it the agent fails at runtime; prep-rest correction, the manager logs it) and `ReadWritePaths=` for
   exactly the daemon config directories its renderers write + `/var/lib/vrx/agent` + `/run/vrx`; api `ReadWritePaths=/var/lib/vrx /data`
   (backups, update bundles — F-backup-restore). F-hardening-lite adds drop-ins later; keep the base units minimal.
5. `nftables` base policy in `vrx-meta`: a static table `inet vrx_base` (allow 22/443 on the management interface only, VPP punt paths,
   drop everything else inbound to the host stack) loaded by `nftables.service` on the appliance. The host-policy renderer (`table inet vrx`)
   is F-host-acl-nftables' (D-057) — never write rules into it. **On this host** only `nft -c` (check) and `nft list` may run in the root
   netns; loading/flushing anything there would cut every session on ens192.
6. Install test: (a) `lintian`, `dpkg-deb -I/-c` on every package — lintian is **not installed on this host** (no host package installs):
   run it inside your build chroot (installed there from the mirror) or record a documented gate; (b) a `debootstrap` Ubuntu 26.04
   (`resolute`) chroot under your worktree's `.scratch/` (or a `systemd-nspawn --private-network` container from it — allowed: it is not
   Docker) installing `vrx-meta` from the local repo, checking unit files, users, the `inet vrx_base` policy (`nft -c` only),
   `systemd-analyze security --offline=yes` and postinst idempotency **without starting VPP** (mask vpp.service in the container; this
   host's VPP must not be touched); (c) the full boot test on a fresh VM is deferred until the product owner provides one — record it as
   deferred in `docs/status/tasks/P10.md`. debootstrap needs the Ubuntu mirror (`repo.amnafzar.ir`), deb.frrouting.org and
   deb.nodesource.com — if unreachable, say so in the questions file (PENDING-network) and finish everything that does not need them.
7. `docs/install/bare-metal.md` and `docs/install/upgrade.md` (package-based for now; A/B
   image is a later task).

## Acceptance
- [ ] `lintian` clean (or documented overrides); packages install, remove, purge, reinstall cleanly (in the chroot/container)
- [ ] `systemd-analyze security --offline=yes` vrx-api ≤ 3.0; agent ≤ 5.0 (pasted)
- [ ] Container boot (`systemd-nspawn -b --private-network`, vpp masked): firstboot runs once, vrx-api/nginx/postgresql/valkey active, the
      rendered startup.conf pasted — the VM reboot test (all services up, config restored by the agent) is **deferred** (no VM, D-002)
- [ ] This host unchanged: root-netns `nft list tables`, `/etc/nginx`, `/etc/systemd/system` identical before/after (pasted)

## Out of scope
ISO installer (P14), A/B upgrade, cloud images, licensing, the nftables host-policy renderer (F-host-acl-nftables), hardening drop-ins
(F-hardening-lite), the strongSwan package (P11), installing anything on this host.
