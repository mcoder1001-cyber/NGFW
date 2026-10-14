# Restore Ethernet names on both appliances

Branch: codex/restore-ethernet-display-dual-host; base main 8eb2f11c4.
Owned scope: integration of completed Ethernet web/UI-kit/CLI aliases from codex/ethernet-display-everywhere (951cbfba9), setup integration tests, deployment/status records. Existing feature worker branches/worktrees remain untouched.

Cause: alias deployment 48ad9f3e9 and completed follow-up PR235 were not merged into main. PR235 was closed targeting the deployment branch. The later setup build based on main lacks the mapping, so installed clients display Linux/engine identities.

Reused completed and independently reviewed feature rather than reimplementing it. Automatic merge with main succeeds; optional WAN, bootstrap administrator adoption, seed opt-in and the protected RA VPN test fixtures are retained. Updated no-WAN integration tests to prove failed global label inventory does not block setup; WAN route discovery stays disabled. Logical configuration keys, PCI identity and management access are preserved; labels change only for human-facing web and CLI views.

Checks pending: UI-kit/web/CLI build and focused tests, unchanged complete quick and hosted gate, independent final review. Then merge into main with expected-head check and deploy the combined build to 172.30.110.211 and 172.30.126.37 without restarting VPP or rewriting network configuration. Original completed feature history remains published on its remote branches.
