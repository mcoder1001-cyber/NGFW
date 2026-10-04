# F-tunnels WIP (2026-10-04)

Branch `codex/next-tunnels-20261004`; worktree `/tmp/ngfw-next-tunnels`; base `dfd001be4`.
Last local contract `4a630ff3b50edfe7ec859f0697f5f8d8734fbb23`; published contract `8bf316183957ac24c15707383a686c6c4e3f1231`, exact tree `99dff12c0ca4f4cb93ce16d2d23a6afb940cdb67` verified by manager. Published source checkpoint local `3c5a4faa178bef1c0930652a69cede60be9f96de`, remote `d8c6ba6db5fcdca601f1a07635e564f194e16f23`, exact tree `851dea8ef957b6fdb1e16eafd297c904b6b5fb7c`. Review corrections follow this file; branch head/Git history provides final local/remote SHA.
Owned: additive proto/generated stubs; agent rpc_tunnels*.go plus service-owned lazy walk gate fields; API features/tunnels plus anchored AgentClient/AppModule; web VPN tunnels and en/fa locales; user docs/task status. Generated API-client is locally regenerated for checks but excluded from source commit (manager integrates combined generation).
Completed: owner-filtered live endpoint/FIB/counter readback; protected REST/client; actual engine-allocation identity UI; VXLAN-GPE and advanced kinds; translated limits/docs; regressions including cached-state suppression after failed polling. Independent reviewer draft concerns fixed: service-owned context-aware gate, preindexed PPPoE join, no green cached status after live-read failure.
Tests: focused Go agent/tunnel descriptors pass; API4/UI6 pass; API/web typechecks pass; targeted API/web lint and proto lint pass. No shared VPP mutations; full CI waived.
Remaining code: none in assigned recovered-source gap scope. Independent review and manager integration pending. Remaining laboratory: dedicated packet/restart/screenshot acceptance and opt-in GTP-U/L2TPv3 mutation.
Current failure: none. Environment inode exhaustion in /tmp resolved by root-backed TMPDIR; use `/root/t-nt` for socket paths.
Next command: `git show --stat HEAD` for independent review, then manager-generated combined API-client and expected-head integration.
