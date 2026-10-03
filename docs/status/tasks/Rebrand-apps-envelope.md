# NGFW application rebrand envelope

Own branch codex/ngfw-rebrand-apps-20261003 in work/NGFW-rebrand-apps; base origin/main0d174caf96413599a6bae7111bf74d14ecebede1.
Own apps/api,apps/web,apps/cli,packages/api-client,packages/ui-kit and unique Rebrand-apps status docs only. Shared schema/proto/yang/agent belong to contract developer; root owns remaining integration/deployment/docs.
User requests full technical/product VRX→NGFW,vrx→ngfw,Vrx→Ngfw naming, including paths, imports, runtime/environment/systemd refs and fixtures. Preserve authentication/session and privilege policies, no host migration/execution. Prior actual compiled checkout retained untouched.
Inventory371 owned files with branding references; rename CLI cmd/vrx,vrx-docgen,vrx-opgen; web vrx-env.d.ts; UI createVrxTheme and VrxThemeProvider files. Generated API/client data must be regenerated from canonical renamed contracts, not diverging manual copies.
Coordinate contract checkpoint before coherent consumer build; no writes to other's worktree/main. Meaningful generation/typecheck/build/unit suites remain mandatory; full hosted integration gate owned by root. No self-review/merge. Publish checkpoints through root connector.
