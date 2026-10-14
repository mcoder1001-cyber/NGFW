# Restore Ethernet names on both appliances

Branch: codex/restore-ethernet-display-dual-host; base main 8eb2f11c4.
Owned scope: integration of completed Ethernet web/UI-kit/CLI aliases from codex/ethernet-display-everywhere (951cbfba9), setup integration tests, deployment/status records. Existing feature worker branches/worktrees remain untouched.

Cause: alias deployment 48ad9f3e9 and completed follow-up PR235 were not merged into main. PR235 was closed targeting the deployment branch. The later setup build based on main lacks the mapping, so installed clients display Linux/engine identities.

Reused completed and independently reviewed feature rather than reimplementing it. Automatic merge with main succeeds; optional WAN, bootstrap administrator adoption, seed opt-in and the protected RA VPN test fixtures are retained. Updated no-WAN integration tests to prove failed global label inventory does not block setup; WAN route discovery stays disabled. Logical configuration keys, PCI identity and management access are preserved; labels change only for human-facing web and CLI views.

Checks pending: UI-kit/web/CLI build and focused tests, unchanged complete quick and hosted gate, independent final review. Then merge into main with expected-head check and deploy the combined build to 172.30.110.211 and 172.30.126.37 without restarting VPP or rewriting network configuration. Original completed feature history remains published on its remote branches.

Focused integration: 26 mapping/provider/control/Setup tests passed, including English/Persian no-WAN flow under failed label discovery; CLI lint/race tests/build passed. Independent integration review found no actionable product issues. Production web build passes bundle budget (523 kB gzip, 600 kB limit) and no-dev-route checks. Both appliance API logins with the documented default account return 401; no further guesses or credential changes were made. Verify rollout via SSH/file hashes and physical PCI mapping; authenticated browser rendering is not claimed. PCI 0000:01:00.0 on 211 is enp1s0f0np0, first in stable port ordering (Ethernet1). PR236 targets main and preserves existing reviewed histories; final CI/merge/rollout results will be recorded in its description.

Complete local quick PASS (11m25s; 681 web tests, 795 API tests). Web/CLI rollout on both hosts verified by HTTPS module/file hashes. Hosted packaging exposed missing CLI directory in prepare fixture; fixed exact build cwd/version flags/output/mode/manifest coverage. Strict packaging PASS: 90 run, no failures/errors/skips. Independent review: no actionable findings. Prior remote e5f197031069c0f46eed300adea44e89e6c3c898; next: push then await final hosted quick/packaging before expected-head squash merge.
