# Independent P10 base-unit packaging review

Frozen `ac827e9a82f46d1f62d71383c656709403b97e08`, remote `aa76368a49f55e1dfa6e3198c02fdc9509ffc58a`. Scope R2/R7/R8 for newly added base-unit restrictions. R4 independently owns runtime syscall/path compatibility; reviewer wrote only this report and ran no generators or services.

R2/R7/R8 APPROVE foundation checkpoint scope, subject to independent R4 results and unchanged complete final hosted gate. No additional capabilities or broad writable paths introduced. API has no capabilities, restrictive namespaces/devices/clock/hostname/kernel access, native syscalls and unchanged private writable state. Agent retains explicitly approved CAP_NET_ADMIN/CAP_SYS_ADMIN/CAP_IPC_LOCK, AF_NETLINK and exact existing writable directories; cgroup namespace restriction does not deny required network namespace operations. MemoryDenyWriteExecute is deliberately absent for Node JIT; agent host/clock/device operations are not blanket-disabled. New hardening directives are understood by actual local systemd 255. No target Ubuntu26 runtime compatibility claim follows solely from parsing.

Actual independent offline score commands:

```text
systemd-analyze --version
systemd 255 (255.4-1ubuntu8.17)
systemd-analyze security --offline=yes deploy/systemd/vrx-api.service
Overall exposure level for vrx-api.service: 3.0 OK
systemd-analyze security --offline=yes deploy/systemd/vrx-agent.service
Overall exposure level for vrx-agent.service: 5.0 MEDIUM
python3 deploy/debian/vrx/tests/test_packaging.py
Ran 8 tests in 0.993s
OK
tools/ci.sh check --base origin/main
ok: gitleaks — scanned ~127855 bytes (127.86 KB) in 220ms no leaks found
check PASSED (0m02s)
```

Both offline numerical prompt thresholds are met locally. Scores measure configured exposure, not working appliance behavior. Direct `systemd-analyze verify` on actual units could not complete because nftables.service and installed product binaries are absent. Diagnostics specifically reported missing nftables.service, /usr/sbin/vrx-agent and /usr/bin/node; no actual installed dependency graph PASS is claimed. Earlier fixture graph proof remains labelled as fixture proof, not target distro boot.

Packaging test adaptations rewrite EUID/path checks only in private test copies to permit fixture execution; shipped root/metadata guards remain. No test assertion was weakened and no shipped root bypass introduced.

Important existing limitations preserved: CAP_CHOWN daemon-UID atomic writer and global identity /etc atomic-parent requirements remain pending in PENDING-P10-agent-file-ownership.md, with no privilege silently expanded. R4 must assess new ProtectKernelTunables against actual renderer/host startup operations before combined approval. Dynamic LCP punt admission remains unbuilt; source licensing blocks release; actual chroot/boot/lifecycle/TLS/API acceptance remains in DEFERRED-ACCEPTANCE. P10 is not marked done. Installer crash durability MINOR belongs to its separate follow-up scope and is not regraded here.
