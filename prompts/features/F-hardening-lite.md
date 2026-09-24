# Task: F-hardening-lite — CIS-lite OS hardening, systemd hardening, package signing   (prepend 00-CONTEXT.md)

## Goal
A pragmatic hardening layer for the appliance (WBS D12.1 in `plan/wbs.csv`, reduced to "lite"): a CIS-inspired baseline for Ubuntu 26.04,
systemd sandboxing drop-ins for every VRX and daemon unit, and verified package signing end to end. The nftables host policy is **consumed**,
not written: F-host-acl-nftables owns the nftables renderer (D-057). Secure boot, pentest and certification are D-059 have-nots.

## Inputs to read first
- `prompts/P10-packaging-deb.md` — units (`vrx-agent` caps `CAP_NET_ADMIN CAP_SYS_ADMIN CAP_IPC_LOCK`; `vrx-api` none, `ProtectSystem=strict`),
  `deploy/apt/` reprepro + Release signing (key in `~/.config/ngfw/apt-signing/`, never committed), `systemd-analyze security` targets (api ≤ 3.0, agent ≤ 5.0)
- `prompts/P14-iso-installer.md` (what the image purges/installs), `docs/09-os-packages.md` §3 (what must not be on the box) and §4 (kernel/boot)
- `prompts/features/F-host-acl-nftables.md` + `apps/agent/internal/renderers/nftables/` (after it merges) — the host policy you rely on
- `docs/decisions/LOG.md` D-001, D-002, D-012 (never restart VPP/daemons on this host), D-057, D-059, D-079 (Kea control sockets, no
  kea-ctrl-agent), D-086 (rsyslog TLS driver), D-100 (nginx :80 redirect only); daemons: FRR, strongSwan (`vrx-strongswan` from P11 — if P11
  has not merged, write proposals only), Kea, Unbound, chrony, snmpd, keepalived, rsyslog
- P10's merged units: the agent needs `AF_NETLINK` (rtnetlink/nft) and `ReadWritePaths` for the daemon configs its renderers write — a
  drop-in that removes them breaks the product; verify every tightening against the unit's real needs in the container
- Host facts (verified 2026-09-24; no host package installs): present — `systemd-analyze` (systemd 259, `--offline=yes`), systemd-nspawn,
  debootstrap (`resolute`), gpg/gpgv, reprepro, shellcheck; absent on the host — auditd, AppArmor userspace (apparmor_parser/aa-status),
  lintian, dpkg-sig, debsig-verify → evaluate those controls inside the chroot/container (packages from the mirror `repo.amnafzar.ir`);
  package signing = signed `Release` + `signed-by=` keyring verified with gpgv (no dpkg-sig). The host's nginx (:80), sshd, PostgreSQL and
  Valkey serve other users — never reload, restart or reconfigure them

## Scope — build exactly this
1. **Baseline** `deploy/hardening/baseline/`: sysctl drop-in (`kernel.kptr_restrict`, `dmesg_restrict`, `fs.protected_*`, `net.ipv4.conf.*.rp_filter`
   for the management side only — never on linux-cp taps, `kernel.unprivileged_bpf_disabled`, …), sshd drop-in (no root password login, no
   password auth when keys exist, ciphers/KEX list), `login.defs`/PAM password quality, `auditd` rules (if the package is in P10's set), cron/at
   restrictions, disabled unused services and kernel modules (`install cramfs /bin/false` style); each item mapped to its CIS control id in a table.
2. **systemd drop-ins** `deploy/hardening/systemd/<unit>.d/10-vrx-hardening.conf` for vrx-agent, vrx-api, nginx, postgresql, valkey and each
   daemon: `NoNewPrivileges`, `ProtectSystem`, `ProtectHome`, `PrivateTmp`, `RestrictAddressFamilies`, `SystemCallFilter=@system-service`,
   `CapabilityBoundingSet` minimal per daemon (FRR needs `CAP_NET_ADMIN CAP_NET_RAW CAP_SYS_ADMIN` for netns — verify each against its real needs).
   Never tighten VPP's unit in this task (D-012; list proposals only).
3. **Package signing** `deploy/hardening/signing/`: verify P10's Release signing and extend it — `debsig`/`dpkg-sig` or signed `Release` + pinned
   `signed-by=` keyring in the APT source; a verify script that fails on an unsigned or wrongly signed repo; key rotation procedure; key material
   only outside the repo.
4. **Audit tool** `deploy/hardening/check.sh`: read-only compliance report (pass/fail per control) usable on the installed box and inside a
   debootstrap chroot; exits non-zero on failures.
5. **Tests**: apply baseline + drop-ins inside a debootstrap Ubuntu 26.04 chroot or `systemd-nspawn` container (allowed; not Docker) with P10's
   packages; `systemd-analyze security --offline=yes` per unit file (pasted scores); `check.sh` output. Nothing is applied to this host.
6. **Docs**: `docs/install/hardening.md` — controls table, exceptions with reasons, how the nftables host policy (F-host-acl-nftables) fits in.

Files you own: `deploy/hardening/**`, `docs/install/hardening.md`, `test/topology/hardening-lite/**`. Shared files: packaging hooks in P10's
`deploy/debian/vrx/**` — one-line additions to install the drop-ins (`vrx-meta` install list, under your anchor), resolved at merge;
`renderers/nftables/**` is F-host-acl-nftables' (read only); P10's `deploy/systemd/vrx-*` base units are read-only (you add drop-ins).
The signing key lives in `~/.config/ngfw/apt-signing/` (P10) — never copy it into the worktree, never print it; rotation is documented and
tested with a throwaway key under `.scratch/`.

## Acceptance (paste the evidence)
- [ ] `systemd-analyze security --offline=yes` scores before/after for every hardened unit, vrx-api ≤ 3.0 and vrx-agent ≤ 5.0 (pasted)
- [ ] `check.sh` in the chroot: all controls pass or are listed exceptions (pasted)
- [ ] APT from the local repo: signed → install OK; tampered `Release` / unknown key → refused (pasted)
- [ ] This host untouched: no changes under `/etc/sysctl.d`, `/etc/ssh`, `/etc/systemd/system` (listing/hashes before/after)
- [ ] `tools/ci.sh --base main` green in your worktree

## Out of scope (do not build)
The nftables host-policy renderer and local-in ACLs (F-host-acl-nftables, D-057); secure boot / signed kernels / TPM measured boot; independent
pentest (D12.3) and certification (D12.7) — D-059 have-nots; air-gapped update bundle (list as have-not); full CIS Level 2; SELinux/AppArmor profile
authoring beyond enabling shipped profiles; VPP unit changes (D-012); A/B upgrade bundle signing (F-ab-upgrade); SECURITY-REVIEW.

## Open questions to surface, not to decide silently
AppArmor: enforce the Ubuntu-shipped profiles for daemons (default) or leave complain mode? Should SSH be restricted to the management VRF/interface by
nftables only (F-host-acl-nftables) or also by `ListenAddress` here — default nftables only.
