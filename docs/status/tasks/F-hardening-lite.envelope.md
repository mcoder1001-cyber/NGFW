# TASK ENVELOPE — F-hardening-lite
id: F-hardening-lite   branch: task/F-hardening-lite   worktree: /root/ngfw-wt/F-hardening-lite   base: main@<BASE>   started: <STARTED>
title: S5 system (day 16-18): CIS-lite OS baseline, systemd hardening drop-ins, package signing verification
prompt: prompts/features/F-hardening-lite.md   (template: prompts/FEATURE-TEMPLATE.md; refreshed on task/prep-rest against main@11a175b: host tools, P10 units need AF_NETLINK, strongSwan via P11)   wbs: D12.1
scope: `deploy/hardening/` baseline (sysctl, sshd, PAM/login.defs, auditd rules if packaged, cron/at, modules) mapped to CIS ids; systemd drop-ins for vrx-agent, vrx-api, nginx, postgresql, valkey and each daemon; signed `Release` + `signed-by=` keyring verification and rotation; `check.sh` compliance report; tests in a chroot / nspawn container with P10's packages. The nftables host policy is consumed, not written (D-057). Nothing is applied to this host
merged deps you can rely on: P10, F-host-acl-nftables (+ P11 soft: the strongSwan unit exists only if P11 merged — else proposals only)
  - P10: `deploy/debian/vrx/`, base units `deploy/systemd/vrx-*` (agent: `AF_NETLINK`, `ReadWritePaths` for the daemon config dirs; api: no caps, `ProtectSystem=strict`), `inet vrx_base`, the repo the manager published to `/srv/vrx-artifacts/apt/`, the signing key location `~/.config/ngfw/apt-signing/`, the SY6 install-list anchor `# wave-BC: F-hardening-lite`
  - F-host-acl-nftables: `apps/agent/internal/renderers/nftables/` (table `inet vrx`, anti-lockout) + docs/agent/renderers/nftables.md
read first: prompts/features/F-hardening-lite.md · docs/status/wave-BC-numbers.md (section "S5 system": SY6, SY7, SY9 + "F-hardening-lite") · docs/install/bare-metal.md (P10) · docs/agent/renderers/nftables.md · docs/09-os-packages.md §3/§4 · docs/decisions/LOG.md D-001, D-002, D-012, D-057, D-059, D-079, D-086, D-100
slot: <SLOT> → VRX_SLOT=<SLOT> VRX_TEST_PREFIX=w<SLOT> VRX_HTTP_PORT=3000+100·<SLOT> VRX_WEB_PORT=5000+100·<SLOT> VRX_METRICS_PORT=9100+10·<SLOT>+1 VRX_AGENT_SOCKET=/run/vrx-test/w<SLOT>/agent.sock VRX_PG_DATABASE=vrx_w<SLOT> VRX_VALKEY_DB=<SLOT> VRX_VPP_TABLE_BASE=<SLOT>000 VRX_LAB_LOCK=/run/lock/vrx-lab.lock
  - source of truth: `eval "$(tools/lab env <SLOT>)"`
  - the chroot/container root, throwaway keys and test repos live under /root/ngfw-wt/F-hardening-lite/.scratch/; nspawn machine name `w<SLOT>-hard`, always `--private-network`
  - slots 1–11 only; 12 is CI
daemon-owner: none on the host. The host's nginx, sshd, PostgreSQL, Valkey, chrony, rsyslog, frr, kea, unbound, snmpd and keepalived serve other users or are owned by other tasks: never reload, restart, enable or reconfigure them. Inside your container every unit is yours
HOST SAFETY:
  - nothing from `deploy/hardening/` is applied to this host: paste sha256 listings of `/etc/sysctl.d`, `/etc/ssh`, `/etc/systemd/system`, `/etc/pam.d`, `/etc/security` before and after
  - in the host's root netns only `nft -c` and `nft list` (HOST FIREWALL SAFETY, as in F-host-acl-nftables)
  - never tighten or edit VPP's unit (D-012): proposals only, in hardening.md
obligations:
  - every tightening is verified against the unit's real needs in the container: the agent needs `AF_NETLINK`, its renderers' config dirs and systemctl over D-Bus; FRR needs `CAP_NET_ADMIN CAP_NET_RAW CAP_SYS_ADMIN` for netns; Kea uses its UNIX control sockets (D-079). A drop-in that breaks a unit is a failed control, not a pass
  - `systemd-analyze security --offline=yes` scores before/after for every hardened unit; vrx-api ≤ 3.0, vrx-agent ≤ 5.0 (pasted)
  - package signing: signed `Release` + pinned `signed-by=` keyring; the verify script fails on an unsigned or wrongly signed repo and on a tampered `Release` (pasted). No dpkg-sig/debsig (not installed). Key rotation is documented and tested with throwaway keys; the real key in `~/.config/ngfw/apt-signing/` is never copied, printed or committed
  - the D-057 split: `inet vrx_base` (P10, static) and `inet vrx` (F-host-acl-nftables renderer) — you add neither rules nor tables; hardening.md explains how they fit
  - sysctl file name `70-vrx-hardening.conf` (after P10's `60-vrx-netlink.conf`, so the hardening file cannot lower `rmem_max` for linux_nl); rp_filter only on the management side, never on linux-cp taps
  - each control is mapped to its CIS id; exceptions are listed with reasons. D-059: secure boot, pentest, certification and full CIS L2 are have-nots
files you own exclusively:
  - deploy/hardening/** (baseline/, systemd/<unit>.d/10-vrx-hardening.conf, signing/, check.sh, tests/run.sh)
  - docs/install/hardening.md
  - test/topology/hardening-lite/** (chroot/nspawn tests; root parts behind `VRX_INTEGRATION=1`, skipped in unit mode)
  - docs/status/tasks/F-hardening-lite*
shared hotspots (append-only, conflicts resolved by the manager at merge; ids from docs/status/wave-BC-numbers.md "S5 system"):
  - protocol: docs/status/wave-A-hotspots.md §0. Insert only directly below `# wave-BC: F-hardening-lite` (seeded by P10). List every hunk under "Shared hunks" in docs/status/tasks/F-hardening-lite.md
  - SY6 deploy/debian/vrx/** (P10's vrx-meta install list): one line per installed hardening path
  - SY7 root .gitignore `/.scratch/`: seeded by the manager; if absent, never `git add -A`
  - SY9 tools/ci.sh: ask for a `deploy/hardening` shellcheck + `tests/run.sh` step; never edit it
contract numbers: none (docs/status/wave-BC-numbers.md "F-hardening-lite": names only)
files you must not touch:
  - everything else
  - never: /root/ngfw (main), other worktrees, /etc, /boot, /root/vpp, host packages or services, `~/.config/ngfw/apt-signing/` contents
  - apps/agent/internal/renderers/nftables/** (F-host-acl-nftables), deploy/systemd/vrx-* base units, deploy/nftables/**, deploy/firstboot/**, deploy/apt/** (P10 — you add drop-ins and verification, never edit the base), deploy/debian/** beyond your anchor lines, deploy/image/** (P14/F-images), deploy/upgrade/** (F-ab-upgrade), deploy/vpp/**, deploy/strongswan/** (P11)
  - apps/**, packages/**, tools/*, plan/tasks.yaml, docs/decisions/LOG.md, docs/status/PROGRESS.md
host facts (verified 2026-09-24; no host package installs):
  - present: systemd-analyze (systemd 259, `--offline=yes`), systemd-nspawn, debootstrap (`resolute`), gpg/gpgv, reprepro, shellcheck
  - absent on the host: auditd, AppArmor userspace (apparmor_parser/aa-status), lintian, dpkg-sig, debsig-verify → evaluate those controls inside the chroot/container (packages from the mirror `repo.amnafzar.ir`); if the mirror is unreachable → PENDING-network in the questions file
coordination:
  - P10: base units and install list; a base unit that cannot reach its score without a base change → questions file (P10 owns the base)
  - F-host-acl-nftables: the SSH/HTTPS management restriction is its renderer's (default: nftables only, no `ListenAddress` here — open question)
  - F-aaa: OS/SSH login policy (PAM-RADIUS out of scope) · F-images/P14: your baseline is not in their images until a follow-up applies it (note it)
evidence: `systemd-analyze security --offline=yes` table before/after, `check.sh` in the container (all pass or listed exceptions), APT signed → install OK / tampered or unknown key → refused, host-unchanged hash listings
time box: 15 h — when exceeded: stop, commit WIP, write docs/status/tasks/F-hardening-lite.md with what is left
WIP: commit at least every 45 min; keep docs/status/tasks/F-hardening-lite-wip.md current
CI: `TMPDIR=/tmp/g-w<SLOT> tools/ci.sh --base main` — short TMPDIR (unix socket paths ≤ 108 chars); no host-wide CI lock: golangci-lint serializes itself since main fc0fe68 (D-106 rejected serialising whole gates). Ports 3000/8080/9101 and /run/vrx/agent.sock belong to the running product stack (tools/app) — never touch them
finish: `tools/ci.sh --base main` green in the worktree · docs/status/tasks/F-hardening-lite.md with pasted real output · everything committed · final message = 10-line summary (branch, last commit, CI result, evidence, open questions, decisions taken with options)
cleanup: no `w<SLOT>-hard` container, no mounts under `.scratch/`, chroot and throwaway keys deleted, processes stopped by PID
questions: docs/status/tasks/F-hardening-lite-questions.md — write and keep going; never wait for a human
never: merge · restart/kill VPP · Docker · pkill · secrets in files · edit files you do not own · apply anything to this host
