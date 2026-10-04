# F-tunnels recovery envelope (2026-10-04)

Branch: codex/next-tunnels-20261004; isolated worktree: /tmp/ngfw-next-tunnels; base dfd001be4.
Owner: tunnels API/client, RPC state, UI/locales/docs, additive proto. Existing advanced projection/descriptors reused; root owns combined generated API client and merge.
User waived full CI. Focused host-independent checks and independent review required. Do not mutate shared VPP, restart VPP, bypass L2TP no-delete or GTP-U dump-first guards. Laboratory packet/restart acceptance deferred.
Gap audit: recovered source already supports advanced schema/projection and owner-filtered TunnelState. Missing REST/UI consumers and live endpoint/FIB/counter completeness.
