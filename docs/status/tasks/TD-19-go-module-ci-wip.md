# TD-19 Go module-version source gate recovery

Own branch `task/TD19-go-module-fixtures-20261002`, isolated
`NGFW-TD19-go-module-fixtures`; frozen base
`be646973db0569b49940707775e7ce087ff5dd9b`. New remote publication awaits manager.
Owned only new workflow/runner and this WIP. Source script/helper/six tests and
original provisioning36 workflow/runner are unchanged; no installer semantics,
main, board, current PR86 or other branch edits.

New readonly pinned-checkout Ubuntu24 workflow requires actual ShellCheck
presence/version and `shellcheck -x -P SCRIPTDIR scripts/20-install-build.sh`,
then strict policy and exactly six named ModuleVersion methods. Exact inventory
refuses empty/missing/extra methods; completion refuses zero/reduced execution,
failure/error/skip/expected failure/unexpected success. Twelve actual unittest
outcome/count and method-inventory controls run separately from six source cases.
Five-minute job, no APT/install/download/root/network/lab setup. Original36 gate
is complementary and remains unchanged; six tests are not called36 PASS.

Actual local 2026-10-02 UTC:12 gate-policy controls PASS;6 source cases PASS0.144s,
zero failures/errors/skips/xfails/xpasses. Shell parser and whitespace checks PASS.
ShellCheck local NOT RUN (unavailable); hosted ShellCheck/six suite, independent
new gate review/current-main composition/full unchanged quick pending. This
new gate is not source/security approval or actual target installation acceptance.
Source review underway separately; no install or real host/network operation.

Next: manager publish checkpoint and dispatch independent gate review; inspect
actual hosted ShellCheck result before claiming PASS, preserve source/provision36
and actual main on final integration, run unchanged complete hosted quick on exact
approved head. No final merge based solely on these local fixture checks.

## Coherent preparation on cleanup main — 2026-10-02 22:32 UTC

Actual main337cbef881ada5bc5ba20aafe396684c4cefd009 is merged in localfeb28877. Sourcebe646973 independently approvedef416dc3; six copied-script cases and eight extra parser/path controls passed. Source/archiveb489c462 and review1f91bd9a, strict-six gateb17b7f5d independently approved496c2ffc and archived5a212a78. Original36 provisioning workflow/runner remains unchanged; new12 gate policies+6 source cases passed. ShellCheck unavailable locally: NOT RUN, hosted ShellCheck/version output+sixcases and full quick remain mandatory. Initial sparse original36 run failure and recovered36 PASS remain attributed source evidence. This preparation does not install anything or mark TD19 whole DONE. Final composition review and refresh against timing-merged main are pending before one-commit integration; no PR86/87/main/board edits.
