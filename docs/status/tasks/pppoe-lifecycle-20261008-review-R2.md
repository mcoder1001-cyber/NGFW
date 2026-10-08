# PPPoE lifecycle security review R2

Reviewed local source `f7aadb6ab7357a73ef259623d7f0e2a2e01f1803`, with clean tracked source. Includes admission fencing, original-parent pidfd capture/inheritance, old pppd stop before replacement, retained retry transition evidence and removal daemon-reload retry. Reviewer did not modify source or independently assert remote publication.

## Security assessment

No outstanding security findings. StopIPv6 validates session/path inputs, persists a blocked admission fence even without PID evidence, and fails closed if helper absence conflicts with process evidence. Replacement admission rotates a cryptographically random token; hook captures it before helper action-lock wait, and helper compares it under that action lock. A queued prior-generation hook cannot acquire the replacement generation. Parent process control is captured as a pidfd and inherited by the refresher, rather than periodically re-opening a possibly recycled numeric PID. Existing identity-checked child stop behavior remains. Supervisor stops old pppd before state invalidation, replacement and reopening; persistent pending evidence supports retries after partial failures. Removed-session admission remains blocked.

Changed daemon actions remain fixed executable paths and argv, with validated host-interface alphabets. Shell hook only reads a fixed renderer-owned admission file and executes a fixed Python helper with quoted arguments; no expanded untrusted shell command. New markers/token are local 0600 files in the existing protected state directory. They are lifecycle generation data, never exported as user credentials. No new API, authentication route, installed dependency, privileged socket or secret-return behavior. Focused private-key/password-pattern scan of task reports found no matches; not a full gitleaks result.

## Independent verification

An offline Python reviewer probe parsed the exact helper template after replacing only fixed renderer constants with synthetic values; compiled it; extracted exact identity/parent_fd/up definitions; and exercised a reviewer-owned temporary parent process plus private synthetic admission files. This runs no renderer, daemon, systemctl, sysctl or packet operation.

```text
PASS inherited pidfd remains tied to terminated original parent
PASS obsolete admission and blocked admission refuse before parent/writer operations
PASS exact helper template syntax after fixed fixture substitution
```

Final narrow delta rechecked: departed-parent up admission returns before state writes; down checks active refresher parent argv before stopping. These preserve replacement ownership when delayed old hooks observe a new token. An additional exact-function probe passed:

```text
PASS departed parent with replacement token ignores without writer operations
PASS final helper template syntax
```

`git diff --check` passed. Go unit/race compilation and native dataplane acceptance were not executed by this reviewer. Source approval does not replace applicable tester reports or unchanged complete quick gate. In particular, these limited probes are not full Go fixture tests or real PPPoE/IPv6 acceptance.

Verdict: **APPROVE** (0 BLOCKER, 0 MAJOR, 0 MINOR), scoped to the exact source SHA above. Product-source changes require recheck.
