# Task envelope
Task: F-system-identity remaining operational state and pre-login banner.
Worker branch/worktree: codex/identity-finish-20261002 / NGFW-identity-finish.
Contract: additive SystemIdentityState RPC and identity field on existing state/system health response.
Security: only configured running login banner is public; max 4096 characters; literal React text.
No privilege, session, secret storage, daemon restart or host hardening changes.
Lab acceptance: centralized DEFERRED-ACCEPTANCE, NOT RUN until access returns.
