# TD-19 installer ShellCheck WIP

Branch: `task/TD19-installer-shellcheck-20261002`; local base `038ee06840e51f823bc69d080f454d5a070948e2`. Remote checkpoint: awaiting manager publication; not yet durable.

Implemented: mandatory ShellCheck presence check, actual version output, unsuppressed `-x -P SCRIPTDIR` check of `scripts/00-add-repos.sh`, `scripts/20-install-build.sh`, `scripts/40-install-lab.sh`, `tools/lab` in provisioning fixtures workflow. Existing suites remain unchanged.

Local ShellCheck: NOT RUN (tool unavailable). Shell parser check and whitespace check recorded at checkpoint. Hosted ShellCheck, provisioning fixtures, unchanged full quick and independent review: NOT RUN. Next: publish checkpoint branch and inspect actual hosted ShellCheck diagnostics; fix genuine findings without suppressions. No provisioning or laboratory execution performed.


Hosted first checkpoint `11e92121`, run `37046775524`: FAIL on 2026-10-02 with ShellCheck 0.9.0. Actual diagnostics SC2016 intentional deferred source/profile literals, SC2015 ambiguous AND/OR lists, SC2001 leftover formatting. Corrected using quoted heredocs, explicit if conditions retaining the success-command failure fallback, Bash newline substitution. No new suppressions. Remote Python fixture extracts the actual new heredoc body; profile regression updated without weakening login-variable requirement. Four added offline regressions execute real changed shell fragments with harmless stubs; all PASS. Latest hosted ShellCheck: pending rerun, NOT PASSED.


## Hosted corrected checkpoint and current-main integration

Corrected source `cb186b45` is durably published as `7e1cfc423bf8cfd907f961a6685740435e151862` on the developer branch. Hosted provisioning run [37047960245](https://github.com/mcoder1001-cyber/NGFW/actions/runs/37047960245) executed ShellCheck **0.9.0** across all four listed files without diagnostics, followed by **36 tests PASS**, zero skips/errors/failures/expected failures/unexpected successes, at **2026-10-02 18:31:59 UTC**. The initial failure above remains historical evidence. Independent source review approved the corrected scope and personally ran 36 fixtures.

Integration is based on actual merged-key main `db15515d21047eac06fdb75e8fc7aaead1a52b5e` and preserves the approved source exactly. Final current-main composition review, one-commit publication and fresh full hosted quick plus provisioning/ShellCheck gates on that exact final head are required before merge. Earlier source-branch runs do not substitute for these final gates. No target installation or laboratory acceptance was executed; whole TD-19 remains incomplete.
