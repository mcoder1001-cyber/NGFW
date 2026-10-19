# LCP unset namespace work in progress

Branch codex/fix-lcp-unset-namespace. Deployed source base 4c8d1b247c6b; prior local checkpoint 2952fe42b. Published plan checkpoint dd07c24f908e109c8efcd159f880439155b1d6ee on the same GitHub branch via authorized connector; CLI push lacks credentials.
Owned files: see task envelope.

Completed: basepolicy malformed namespace read now requires independently corroborated exact unset status via generated fixed read-only CliInband. Existing API validity/error behavior preserved. Nineteen table-driven regression cases cover fail-closed behavior, command identity and context. Independent implementation review APPROVE (read-only; test execution not yet verified).

Tests: not yet executed. Official Go1.26.0 archive download remains in progress; expected SHA256 aac1b08a0fb0c4e0a7c1555beb7b59180b05dfc5a3d62e40e9de90cd42f88235. No production binary deployed. No VPP restart, namespace policy change or user draft modification.

Additional observed restart prerequisite: existing API omits NGFW_AGENT_OWNER, so disabled auto-block writes an empty cache which old agent refuses on startup. Before agent replacement, preserve the cache, explicitly configure the API's existing ngfw owner, and initialize only an observed empty cache with that same owner. No new owner or privileges.

Remaining: format/test/build, publish tested checkpoint, back up and deploy matching agent binary, verify malformed namespace no longer blocks projection. Preserve the user's staged credential/firewall changes; do not apply them merely to test this repair.
Next command: /tmp/ngfw-lcp-toolchain/go/bin/go test ./internal/subsystems -run TestBasePolicy -count=1 (from apps/agent, when download completes).
