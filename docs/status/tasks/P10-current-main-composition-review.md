# Independent current-main composition review

Reviewed isolated local integration `48ec1e1cf3832212ce0a5b55eb85ece89979f469`, tree `ac10239bef532b22de1ebed48e4a4ac9078f73e2`, merging reviewed P10 source `25e02a8f86ae15538d3aceb647e057e0ae46c646` with dashboard main `c76774e8d2e04abee8ea7a301e64acbb3f794373`. Fresh GitHub ref independently confirms main remains c767.

APPROVE concrete source composition, conditional on final single-commit transport atop current main and fresh unchanged full hosted quick on its exact resulting tree. No hosted PASS or whole P10 acceptance claimed.

Independent blob comparison confirms all 39 P10-changed files outside overlaps exactly match reviewed P10 source; all 31 main dashboard changed files outside overlaps exactly match main. Only shared overlaps are decision LOG and subsystem registration. LOG preserves both D164 dashboard decision and D172/D173/D174; subsystem file preserves dashboard management domain/registration while adding the exact three reviewed registerBasePolicy lines. No dashboard code or metadata lost. API/web/board/progress and CI/gates have no unrelated difference from new main. Existing campaign/source appendix and preserved findings retain exact reviewed state.

Actual targeted race checks of the newly composed subsystem package:

```text
go test -race -count=1 ./internal/subsystems -run 'TestBasePolicy|TestReachabilityTable|TestDashboardProm'
ok ngfw/agent/internal/subsystems 1.910s
go test -race -count=1 ./internal/subsystems -run '^TestPrometheusListenerLifecycle$'
ok ngfw/agent/internal/subsystems 1.059s
git diff --check
exit 0
```

The first command's matching checks cover basepolicy and reachability; actual dashboard lifecycle test has a different name and was explicitly run by the second command. It uses its own ephemeral loopback test listener, not a production service. This validates the shared compiled registration package without repeating whole CI. Final remote head/base SHA and full hosted quick must still be checked after publication; if main changes, reverify composition. No source edits, live host config or package installation occurred.
