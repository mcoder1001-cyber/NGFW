# TD-19 installer ShellCheck WIP

Branch: `task/TD19-installer-shellcheck-20261002`; local base `038ee06840e51f823bc69d080f454d5a070948e2`. Remote checkpoint: awaiting manager publication; not yet durable.

Implemented: mandatory ShellCheck presence check, actual version output, unsuppressed `-x -P SCRIPTDIR` check of `scripts/00-add-repos.sh`, `scripts/20-install-build.sh`, `scripts/40-install-lab.sh`, `tools/lab` in provisioning fixtures workflow. Existing suites remain unchanged.

Local ShellCheck: NOT RUN (tool unavailable). Shell parser check and whitespace check recorded at checkpoint. Hosted ShellCheck, provisioning fixtures, unchanged full quick and independent review: NOT RUN. Next: publish checkpoint branch and inspect actual hosted ShellCheck diagnostics; fix genuine findings without suppressions. No provisioning or laboratory execution performed.


Hosted first checkpoint `11e92121`, run `37046775524`: FAIL on 2026-10-02 with ShellCheck 0.9.0. Actual diagnostics SC2016 intentional deferred source/profile literals, SC2015 ambiguous AND/OR lists, SC2001 leftover formatting. Corrected using quoted heredocs, explicit if conditions retaining the success-command failure fallback, Bash newline substitution. No new suppressions. Remote Python fixture extracts the actual new heredoc body; profile regression updated without weakening login-variable requirement. Four added offline regressions execute real changed shell fragments with harmless stubs; all PASS. Latest hosted ShellCheck: pending rerun, NOT PASSED.
