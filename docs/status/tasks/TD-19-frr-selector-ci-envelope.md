# TD-19 selector hosted gate envelope

Own isolated `NGFW-TD19-frr-selector-ci`, branch
`task/TD19-frr-selector-ci-20261002`, exact frozen source base
`763ce40ba29c675e6f6b4505abc2f5a3afb441ef`. Own new selector workflow,
strict runner, bounded policy controls and these own CI envelope/WIP only.
Existing source helper, selector fixtures, old36/raw9/newGo6 workflows/runner,
main, board, PR88 and official/operator trust pins remain untouched.

Ubuntu24.04 runner with read-only contents permission, pinned existing checkout,
credentials disabled, ten-minute timeout. Report actual Python/GPG/gpgv/gpgconf/
ShellCheck versions before source shellcheck. No package installation/network
bootstrap/upstream-key retrieval or service calls. Generated signing material is
solely existing temporary test factory data; no external A90 trust, release key,
global GPG home or production installation. Runner pin is not Ubuntu26.04
appliance acceptance. Fixture factory can run only on a host where GPG works.

Frozen exact13 = six ControlledSelection methods plus seven RealCertificates
methods. Runner hardcodes every fully qualified name; Counter equality rejects
missing, reduced, extra, duplicate or zero inventory before execution. Result
checks started13 and all13 actual stopTest completions, ordinary unittest result,
zero failures/errors/skips/expectedFailures/unexpectedSuccesses. Class setup
errors cannot be treated as completion. Methods run once; no repeat real factory
as part of policy controls. Five policy-control methods use isolated named test
cases to exercise refusals without factory or production selector execution.

Known local source evidence supplied by manager: controlled6 PASS; previous
real-factory attempt5 PASS plus setupERROR because GPG-agent unavailable.
Seven real cases remain NOT RUN; local full13 positive is NOT PASS.
Do not rerun the blocked local certificate-generation factory or skip tests.
Hosted exact13 positive, independent CI review and unchanged full host gate
are mandatory before merge. This CI source phase does not finish TD-19.
