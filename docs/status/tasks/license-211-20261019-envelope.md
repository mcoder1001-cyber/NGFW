# Appliance 211 licence activation — task envelope

- Request: product owner asked in chat to activate licences on 172.30.110.211.
- Worker: root; branch `codex/license-211-20261019`; worktree `/root/ngfw-wt/license-211-20261019`.
- Owned files: this envelope and `docs/status/tasks/license-211-20261019-wip.md` only.
- Scope: inspect current licensing, issue a signed full-feature 365-day licence with the documented support tool if needed, configure its public-key trust, upload through authenticated HTTPS API, and verify persistent valid status.
- No product code changes or product merge. Private signing material stays outside Git and off the appliance.
- Preserve current routing/L2 configuration and existing trusted keys. Only the API service may need restart; no VPP or agent restart.
- Independent reviewer: `/root/routing_review`, read-only review of documentation, plan and verification.
- Supported workflow: `docs/user/system/licensing.md`, `tools/license/generate-license.sh`, `tools/license/ngfw-license.mjs`. This is the owner's documented issuance/install operation, not a change to the licensing or authentication model.

