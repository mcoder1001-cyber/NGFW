# Restore Ethernet names on both appliances

The implementation and rollout entries below are historical task records. Current source/CI closure is recorded at the end; this recovery did not contact the appliances.

Branch: codex/restore-ethernet-display-dual-host; base main 8eb2f11c4.
Owned scope: integration of completed Ethernet web/UI-kit/CLI aliases from codex/ethernet-display-everywhere (951cbfba9), setup integration tests, deployment/status records. Existing feature worker branches/worktrees remain untouched.

Cause: alias deployment 48ad9f3e9 and completed follow-up PR235 were not merged into main. PR235 was closed targeting the deployment branch. The later setup build based on main lacks the mapping, so installed clients display Linux/engine identities.

Reused completed and independently reviewed feature rather than reimplementing it. Automatic merge with main succeeds; optional WAN, bootstrap administrator adoption, seed opt-in and the protected RA VPN test fixtures are retained. Updated no-WAN integration tests to prove failed global label inventory does not block setup; WAN route discovery stays disabled. Logical configuration keys, PCI identity and management access are preserved; labels change only for human-facing web and CLI views.

Checks pending: UI-kit/web/CLI build and focused tests, unchanged complete quick and hosted gate, independent final review. Then merge into main with expected-head check and deploy the combined build to 172.30.110.211 and 172.30.126.37 without restarting VPP or rewriting network configuration. Original completed feature history remains published on its remote branches.

Focused integration: 26 mapping/provider/control/Setup tests passed, including English/Persian no-WAN flow under failed label discovery; CLI lint/race tests/build passed. Independent integration review found no actionable product issues. Production web build passes bundle budget (523 kB gzip, 600 kB limit) and no-dev-route checks. Both appliance API logins with the documented default account return 401; no further guesses or credential changes were made. Verify rollout via SSH/file hashes and physical PCI mapping; authenticated browser rendering is not claimed. PCI 0000:01:00.0 on 211 is enp1s0f0np0, first in stable port ordering (Ethernet1). PR236 targets main and preserves existing reviewed histories; final CI/merge/rollout results will be recorded in its description.

Complete local quick PASS (11m25s; 681 web tests, 795 API tests). Web/CLI rollout on both hosts verified by HTTPS module/file hashes. Hosted packaging exposed missing CLI directory in prepare fixture; fixed exact build cwd/version flags/output/mode/manifest coverage. Strict packaging PASS: 90 run, no failures/errors/skips. Independent review: no actionable findings. Prior remote e5f197031069c0f46eed300adea44e89e6c3c898; next: push then await final hosted quick/packaging before expected-head squash merge.

## Completed integration

Final reviewed source head22da97d805d9272393bf7f59499e5f221aea2d96 passed hosted mandatory quick21m6s and both packaging checks. Expected-base8eb2f11c4 and expected-head22da97d805 checks succeeded; PR236 squash merged into main4f70b3315b9641738f9d193bbad6fb271c9a3d3f. Fetched integration tree exactly equals tested head (git diff --exit-code HEAD origin/main). Reviewed source history remains in remote branches and local refs/archive/restore-ethernet-display-dual-host-20261014. Main quick/packaging started runs38590496159/38590496148; their eventual result is not yet claimed.

Both installed CLIs now report ngfw4f70b3315b96, binary SHA5736600f7e3ec17de3e59c95cd9c0c927e06a8d029d90468e76e431bbb99b4c2 matches local merged-source build. Web module hashes remain verified. API,agent,VPP,nginx all active. Independent operational tuning/boot persistence completed on both devices; recovery/source records published on codex/appliance-half-resource-defaults (fc1fcaa4a). No remaining feature code or merge step. Next diagnostic command: gh run view38590496159; authenticated browser/full-OS-reboot/packet-throughput acceptance remains unperformed, not a failed quick gate.

## Recovered final status and verified main CI — 2026-10-14

The completed-integration paragraphs above were recovered from `codex/restore-ethernet-display-dual-host` at `80810da304f9d5eb7b251aafdb3855c7dfe3c592`. Comparing that source tree with main `4f70b3315b9641738f9d193bbad6fb271c9a3d3f` shows only those six added status lines; there is no outstanding feature delta to merge again. PR236 already integrated the product code. Existing source/checkpoint branches remain preserved.

Fresh GitHub connector reads confirm these **main** workflow results for exactly `4f70b3315b9641738f9d193bbad6fb271c9a3d3f`:

| Workflow | Run | Status / conclusion | GitHub updated timestamp (UTC) |
| --- | --- | --- | --- |
| CI gate | [38590496159](https://github.com/mcoder1001-cyber/NGFW/actions/runs/38590496159) | `completed` / `success` | 2026-10-14 15:54:17 |
| Packaging fixture gate | [38590496148](https://github.com/mcoder1001-cyber/NGFW/actions/runs/38590496148) | `completed` / `success` | 2026-10-14 15:34:57 |

This supersedes the historical pending-main-CI line. These receipts cover the existing main commit, not a later recovery-branch integration. The unchanged complete hosted quick gate is still required for that integration.

The earlier CLI hashes, module hashes and service-health observations are recovered evidence, not fresh SSH or browser verification. Authenticated browser rendering, full operating-system reboot acceptance and packet/throughput testing remain NOT RUN. Operational resource records are preserved in [appliance-half-resource-defaults-wip.md](appliance-half-resource-defaults-wip.md), with the scripts archived as text and their replay limitations documented. No new authentication route or live operation was introduced by this recovery.
