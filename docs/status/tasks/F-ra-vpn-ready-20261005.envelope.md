# F-ra-vpn recovery execution envelope
Branch codex/ready-f-ra-vpn-20261005; worktree /root/ngfw-wt/ready-f-ra-vpn-20261005; base7b507db5; slot19.
Owner direction2026-10-05 via manager supersedes retired strongSwan/kernel-vpp prompt: native-only and no VPP C changes/security-boundary changes. Investigate concrete native capabilities; implement honest failclosed validation/API/UI while source dependency is unavailable. Remote-access EAP/pools/session service is NOT claimed operational.
Owned: new semantic/ra-vpn.ts/test, desired/ra_vpn.go/test, apps/api/src/features/ra-vpn/**, apps/web/src/domains/vpn/ra-vpn/**, locales/*/ra-vpn.json, docs/user/vpn/ra-vpn.md, own status and PENDING-native-ra-vpn.md.
Shared authorized: semantic index import/spread; projection guard call; remove own retired unsupported-warning leaf; app.module import/controller; VPN tab+i18n anchors; generated outputs through pnpm gen only. Additive contract commits first. No agent core/privilege/VPP binding/renderer changes.
No daemon owner, host objects/services/packages or lab changes. Root owns independent review/merge/board. Disabled profiles may remain editable draft documentation, but enabling unsupported profiles must fail before any runtime write. Retired implementation requires separate architectural/security approval.


2026-10-05 later explicit owner answer supersedes the earlier native-only boundary:
«طراحی و پیاده‌سازی موتور مستقل strongSwan برای VPN دسترسی از راه دور، با بازبینی امنیتی و تست کامل».
Authority now covers independent private namespace/kernel-netlink XFRM RA, with
selected VPP underlay/protected TAP handoff and explicit ingress/egress policy;
no retired kernel-vpp revival/native S2S ownership change. Necessary owned source
extends to internal/ra_vpn/**, command ngfw-ra-daemon, own RA systemd template,
minimal packaging staging/install/dependency hunks, scheduler/projection and sealed
resolver anchors, generated contracts, topology acceptance and decision LOG.
Networking/daemon tests must be disposable own-slot fixtures; no shared host
packages/services/sysctls or main VPP mutation. Private daemon gets NET_ADMIN,
NET_BIND_SERVICE and IPC_LOCK only, no SYS_ADMIN. Enabled guard stays until runtime
verified. Task is explicitly incomplete until engine/security/tests/API/UI done.
Current contract17 transport,18 accessPolicy is published84050494; source owner
RA VICI/renderer checkpoints7a5e2489,6b54ad8d. Planned helper verifies namespace
inode and fixed capability boundary before only namespace networking operations.
