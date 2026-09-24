# F-startup-apply — WIP

- [x] `git merge main` (main had dropped deploy/vpp/*; old script + test restored from a8126ae, `.pyc` and `vpp-iface-check.py` not restored)
- [x] N3: `apps/agent/cmd/vrx-vppcheck` (version / plugins / ifaces via the agent's VPP client, hard deadline) + unit tests incl. hung socket; one real read-only run on vrx-a
- [x] apply-startup.sh: N1 timeouts + dead-man kill/bounded locks/forced rollback, N2 snapshot/restore + netmgr detection, N5 rendered sha pin + pinned binaries, N6 env via systemd-run --setenv, handover gate, N9 fd close, N11 reset-failed, plugin-omission exemption
- [x] fake-host test: hung VPP, SSH death mid-apply, ifupdown restore, networkd/netplan, lock contention, dead-man kills hung run
- [x] docs/agent/renderers/vppstartup.md procedure
- [x] CI + status file
- status: finished, see F-startup-apply.md
