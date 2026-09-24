# Task P10 — Debian packaging + systemd + install script   (prepend 00-CONTEXT.md)

## Goal
Turn the three apps into signed `.deb` packages that install on a clean **Ubuntu 26.04** and
come up as a working appliance, driven by systemd, with the correct privilege split. VPP comes from our
source-built packages in `/root/vpp/build-root/*.deb` (D-001), never from the FD.io repo.

## Read first
`docs/09-os-packages.md`, `scripts/10-install-runtime.sh`, `docs/01-architecture.md` AD-6.

## Build exactly this
1. Packages: `vrx-agent` (static Go binary, root, `After=vpp.service`, `Requires=vpp.service`,
   `Restart=always`, creates `/run/vrx` and `/var/lib/vrx/agent`), `vrx-api` (bundled Node app
   via `pnpm deploy` + system `nodejs` 22 dependency, user `vrx`, no capabilities,
   `ProtectSystem=strict`, `NoNewPrivileges=yes`), `vrx-web` (built SPA + nginx site config +
   TLS bootstrap with a self-signed cert on first boot), `vrx-meta` (depends on all + vpp (= 26.06-release, ours),
   frr, `postgresql (>= 16)`, valkey (or redis-server), nginx; ships `/etc/vrx/startup.conf.tmpl` and the
   `vrx-firstboot.service` that creates the DB, runs migrations, seeds the admin from
   `/etc/vrx/bootstrap.env`, then disables itself).
2. `deploy/debian/` with proper `debian/control`, `rules` (dh), `postinst/prerm` that are
   idempotent and never destroy data on upgrade; versions from git tags (`0.1.0~dev+<sha>`).
3. `deploy/apt/`: `reprepro` config + `Release` signing with a GPG key generated locally into `~/.config/ngfw/apt-signing/` (never committed);
   `scripts/publish-apt.sh`; include our VPP debs in the repo.
4. `deploy/systemd/` hardening: agent unit gets `CapabilityBoundingSet=CAP_NET_ADMIN
   CAP_SYS_ADMIN CAP_IPC_LOCK` and nothing more; api unit gets none; both `PrivateTmp`,
   `ProtectHome`, `RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6`.
5. `nftables` host policy in `vrx-meta`: allow 22/443 on management interface only, VPP
   punt paths, drop everything else inbound to the host stack.
6. Install test: (a) `lintian`, `dpkg-deb -I/-c` on every package; (b) a `debootstrap` Ubuntu 26.04 chroot (or systemd-nspawn container from it —
   allowed: it is not Docker) installing `vrx-meta` from the local repo, checking unit files, users, nftables policy and postinst
   idempotency **without starting VPP** (this host's VPP must not be touched); (c) the full boot test on a fresh VM is deferred until the
   product owner provides one — record it as deferred in `docs/status/tasks/P10.md`. startup.conf template supports `dpdk { no-pci }`.
7. `docs/install/bare-metal.md` and `docs/install/upgrade.md` (package-based for now; A/B
   image is a later task).

## Acceptance
- [ ] `lintian` clean (or documented overrides); packages install, remove, purge, reinstall cleanly
- [ ] `systemd-analyze security vrx-api.service` ≤ 3.0; agent ≤ 5.0
- [ ] Reboot the VM → all services up without intervention → config restored by agent

## Out of scope
ISO installer (P14), A/B upgrade, cloud images, licensing.
