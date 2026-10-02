# F-system-identity continuation contract
Contract-first commit: c1bb711c; generated consumer commit: 4ee854d2.
Additive SystemIdentityState RPC, no changed/renamed fields or health routes.
Response: descriptor-scoped installed identity, bounded optional host facts and explicitly separate configured/observed DNS.
Errors carry fixed field identifiers, no file contents. Owner mismatch rejected; missing wiring unavailable.
API GET /state/system retains existing fields and adds nullable identity. Public GET /auth/banner exposes only committed
login text, max4096 UTF-16 code units without split surrogate, no-store, no candidate/motd/hostname/user/secret fields.
UI escapes literal text, survives banner failure, shows state error/unavailable distinctly, uses shared number formatters.
Pending host privilege/restart boundaries unchanged. Lab NOT RUN; see DEFERRED-ACCEPTANCE.md.

Ordinary choices (central decision LOG D-174): preserve existing health-route shape and add nullable identity
rather than replace it; use fixed read-only resolver runtime observations instead of privileged daemon commands; return only
committed banner through a narrow public API rather than expose config or route login through the agent. Alternatives cost
more or cross pending privilege boundaries. Reversal estimate <1 hour, continuation task estimate ~3 hours.
