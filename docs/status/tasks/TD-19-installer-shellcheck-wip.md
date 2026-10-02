# TD-19 installer ShellCheck WIP

Branch: `task/TD19-installer-shellcheck-20261002`; local base `038ee06840e51f823bc69d080f454d5a070948e2`. Remote checkpoint: awaiting manager publication; not yet durable.

Implemented: mandatory ShellCheck presence check, actual version output, unsuppressed `-x -P SCRIPTDIR` check of `scripts/00-add-repos.sh`, `scripts/20-install-build.sh`, `scripts/40-install-lab.sh`, `tools/lab` in provisioning fixtures workflow. Existing suites remain unchanged.

Local ShellCheck: NOT RUN (tool unavailable). Shell parser check and whitespace check recorded at checkpoint. Hosted ShellCheck, provisioning fixtures, unchanged full quick and independent review: NOT RUN. Next: publish checkpoint branch and inspect actual hosted ShellCheck diagnostics; fix genuine findings without suppressions. No provisioning or laboratory execution performed.
