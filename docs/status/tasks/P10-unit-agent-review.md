# P10 unit delta — independent R4 / R2 review

Reviewed local `ac827e9a`, remote `aa76368a49f55e1dfa6e3198c02fdc9509ffc58a`, against `2d192cfe`. Scope: the new API/agent systemd sandbox directives, not whole P10 acceptance.

## Verdict

**APPROVE the unit hardening delta.** No new BLOCKER or MAJOR found by source/call-graph inspection. This is not approval of installed appliance functionality or whole P10 completion. The capability/file ownership and atomic global-file parent mismatches below remain explicit PENDING decisions.

## Compatibility and privilege inspection

- Agent retains `CAP_NET_ADMIN CAP_SYS_ADMIN CAP_IPC_LOCK`, AF_UNIX/INET/INET6/NETLINK, the host network namespace, writable daemon directories, and no PrivateDevices or ProtectHostname restriction. Production rtnetlink/nft calls remain admissible; `nftables/runner.go` uses CLONE_NEWNET setns, while `RestrictNamespaces=~cgroup` denies only cgroup namespace creation/entry. System identity SetHostname retains its existing CAP_SYS_ADMIN boundary.
- Production agent host facts, boot identity and startup-preview RPCs read `/sys` and `/proc`; ProtectKernelTunables makes these kernel tunable areas read-only rather than hiding their read interfaces. No production write to sysfs/proc tunables or direct module insertion was found in the inspected Go call graph.
- Audited renderer ALLOWLIST and execution controller boundaries: agent invokes fixed FRR validation/reload, daemon config checkers, bounded journal queries, nft, and systemctl controller operations. Real daemons are separate systemd services; chrony time adjustment and PPP /dev/ppp access do not run inside the API sandbox. NativeABI, realtime, personality and SUID restrictions have no identified production requirement in these calls.
- `rpc_dataplane_startup.go` is read-only. ALLOWLIST explicitly says VPP startup apply/restart is a manager step. `deploy/vpp/apply-startup.sh` driverctl/sysfs mutation runs through separate transient systemd units; this reviewed delta does not place those units inside the agent sandbox or execute them. No VPP restart, root-netns nft load, host package installation, or live systemd modification was attempted.
- API uses Node 22 and ordinary TCP/Unix sockets. It does not execute dataplane helper processes, create namespaces, adjust clocks/hostname, use privileged hardware devices or require realtime scheduling in inspected production sources. PrivateDevices retains standard pseudo-devices. MemoryDenyWriteExecute was deliberately not added, preserving Node JIT. Its existing empty capability set is unchanged.
- No binary API/generated contract or new writable path/capability is added by the reviewed unit delta. No new credentials, user-input shell execution or dependency is introduced.

## Existing unresolved boundaries — not waived

1. `helpers_files.go` creates temporary files and calls os.Lchown for fixed daemon owners. CAP_CHOWN is missing from the approved agent set; installed FRR/Kea/chrony ownership cannot be claimed working. Do not silently broaden capabilities.
2. Atomic updates to `/etc/hostname`, `/etc/localtime`, banners etc. need writable parent directories; strict filesystem protection and narrow current paths do not supply them. Do not silently add broad writable `/etc`.

Both are documented in `docs/decisions/PENDING-P10-agent-file-ownership.md`. This review preserves them; laboratory deferral is not a privilege decision.

## Verification personally executed

`python3 -m unittest discover -s deploy/debian/vrx/tests -p 'test_packaging.py'`

```
........
Ran 8 tests in 1.674s
OK
```

Read AGENTS, task P10, shared review rules, R2/R4 prompts, context, decision policy, shared-host rules, unit diff, renderer allowlist, production Go syscall/helper call sites and API process/device references. Static inspection establishes compatibility intent, not real sandbox execution. Actual Ubuntu 26.04 boot, daemon reload, journal access, agent reconciliation and API health under installed units: **NOT RUN**, retain centralized deferred acceptance. The other reviewer owns offline score/operability verification.
