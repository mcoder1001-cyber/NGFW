# LCP unset namespace work in progress

Branch codex/fix-lcp-unset-namespace. Deployed source base 4c8d1b247c6b; prior local checkpoint 2952fe42b. Published plan checkpoint dd07c24f908e109c8efcd159f880439155b1d6ee on the same GitHub branch via authorized connector; CLI push lacks credentials.
Owned files: see task envelope.

Completed: basepolicy malformed namespace read now requires independently corroborated exact unset status via generated fixed read-only CliInband. Existing API validity/error behavior preserved. Nineteen table-driven regression cases cover fail-closed behavior, command identity and context. Independent implementation review APPROVE (read-only; test execution not yet verified).

Tests: focused TestBasePolicy PASS; complete basepolicy renderer and LCP descriptor packages PASS; go vet of subsystems/basepolicy/LCP/agent PASS; static agent build PASS. Race run pending. Broader subsystems+agent package tests FAIL on pre-existing fixture directory permissions; unmodified deployed baseline independently reproduces TestRegisterACLWiring failure. Initial new test compile failed because this generated namespace reply has no Retval; corrected test to the checked generated schema, then focused tests passed. Official Go1.26.0 archive downloaded and checksum verified; expected SHA256 aac1b08a0fb0c4e0a7c1555beb7b59180b05dfc5a3d62e40e9de90cd42f88235. No production binary deployed. Source checkpoint 89522895f031e08567ba863c94eb8cacd74c863e is published; its tree 3356c6b03771331fe1d073eb83116a1c949b0846 equals local f2ad1ea5647cd64b91f3bb09ce24d16007a79603 tree. Final delta adds C1 control rejection and corrects test-only schema use; independently reviewed APPROVE. No VPP restart, namespace policy change or user draft modification.

Additional observed restart prerequisite: existing API omits NGFW_AGENT_OWNER, so disabled auto-block writes an empty cache which old agent refuses on startup. Before agent replacement, preserve the cache, explicitly configure the API's existing ngfw owner, and initialize only an observed empty cache with that same owner. No new owner or privileges.

Remaining: finish race test, publish tested checkpoint, back up and deploy matching agent binary, verify malformed namespace no longer blocks projection. Preserve the user's staged credential/firewall changes; do not apply them merely to test this repair.
Next command: wait for race-focused TestBasePolicy; rebuild stripped static agent and deploy with backup, then issue read-only repeated DryRun requests against unchanged candidate.
