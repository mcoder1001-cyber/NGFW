# TD19 source fixture compatibility handoff

Branch codex/td19-source-fixture-compat-20261007; worktree /root/ngfw-wt/td19-source-fixture-compat-20261007; actual main base4fcdc4557e857efb8d079bf2c52c0e2f0fedd9db. Frozen reviewed repairf745de9c3e6344cdf4a459b824f96722743c42c0 successfully published. PR https://github.com/mcoder1001-cyber/NGFW/pull/207; root confirmed native artifact attached. Only two assigned fixturefiles and taskdocs changed; products/workflows/runners untouched.

## Reproduction and correction

Exact unchanged Go runner baseline6tests/1FAIL0.996s; trusted absolute /usr/bin/awk ignored obsolete PATH interception, then copied entry lacked shared helper. Copy real helper/recorder and replace exactly one AWK reader subprocess in the disposable entry with an owned failing reader. Preserve actual AWK program/canonical module input/refusal branch, both original required diagnostics and no-calls assertion.

Exact unchanged FRR runner baseline13started/completed, two adverse subcasesFAIL16.730s; full-source bash-c entry could not resolve shared helper and never reached required import/Node show-key gates. Execute copied entry/helpers from disposable repo, original trusted helper/recorder byte copies, recording-harness PATH and fake root. Inject exactly eight owned GPG subprocess callsites to the controlled mock outside effect PATH, while original trust/parser/import gates and helper/harness enforcement run unchanged. Explicit existing artifact preflight fixture model remains. Preserve no-host/private-home cleanup and reached-gate assertions; additionally assert node.key-specific show-key and downloaded-NodeSource refusal, FRR failure before Node, no APT/keyring/list output.

## Actual tests and review

Commands: TMPDIR=/tdc PYTHONDONTWRITEBYTECODE=1 python3 .github/scripts/go-module-version-fixtures.py; sameenv python3 docs/status/tasks/TD-19-run-frr-selection.py. Unchanged exact Go6PASS0.974s: failures/errors/skips/expectedFailures/unexpectedSuccesses0. Final exact FRR13 started/completed13PASS23.579s: all adverse/real certificate cases complete, failures/errors/skips/expectedFailures/unexpectedSuccesses0, accepted=True. Unchanged Go policy12PASS. No inventory/count/skip reductions. git diff --check PASS.

Independent APPROVE exactf745 published5cd6c7cf083cceb343f7b44a47dc0443cf73f81a: exactmodule6PASS0.965s/FRR13PASS25.082s, policy12+5PASS, no products/runners/workflow changes, ShellCheckclean. No source fixture failure remains.

## Handoff

Final containing report-only local/remote SHAs resolve via git rev-parse HEAD and git ls-remote origin refs/heads/codex/td19-source-fixture-compat-20261007; this receipt does not change the reviewed two fixtures. Branch read-only after successful publication; developer will not push without manager coordination. Manager archives reviewed history, integrates/merges with expected current head and unchanged actualmain check. Exact next: git show origin/codex/td19-source-fixture-compat-20261007:docs/status/tasks/td19-source-fixture-compat-20261007-report.md.

Fresh aggregate CI explicitly remains waived; no downloads/installs/native or live acceptance performed. This bounded compatibility correction does not close TD19 release Python-lock closure or real target provisioning/boot acceptance.
