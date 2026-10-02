# Independent P10 packaging checkpoint review

Reviewed frozen local `ff6223db30aa6648bb05f5d262d71dd892a1ea94`, identical product tree remote `c401f4efde321505cdd6a358cf93d044b1d215b4`. Scope R1/R2/R7/R8. Reviewer owns this report only; no product edits, service starts, package installs or host configuration changes.

## Findings

1. **MAJOR — installed agent sandbox blocks implemented runtime features.** `deploy/systemd/vrx-agent.service:28-29` enables `ProtectSystem=strict` but permits neither `/etc/vrx/rsyslog-tls` (actual rsyslog renderer TLS path in `apps/agent/internal/renderers/rsyslog/paths.go`) nor `/var/lib/vrx/captures` (actual capture default in `apps/agent/internal/actions/capture-trace/capture.go`). Thus TLS config writes and capture persistence fail under the product service even when code and permissions otherwise work. Provision precise root-owned directories with correct modes and add narrow ReadWritePaths, or explicitly configure existing consumers to permitted paths. Add regression coverage against actual runtime path contracts; do not broadly permit `/etc`.
2. **MAJOR — startup apply binary locations disagree with shipped script.** `deploy/debian/vrx/prepare.sh:34-36` and `debian/vrx-agent.install` place startupgen/vppcheck under `/usr/sbin`. Shipped `deploy/vpp/apply-startup.sh:99,205` product mode defaults to `/usr/lib/vrx/bin` and fails binary preflight. Install tools in the documented directory or supply a packaged wrapper/config consistently setting VRX_LIB_BIN to their actual location; verify normal documented product invocation without caller-specific workarounds.

R1 BLOCK; R8 BLOCK until these findings are fixed. R2 APPROVE checkpoint security scope: no new secrets, auth boundaries, dependency licenses or host changes; the approved capability split and AF_NETLINK are preserved, API has zero capabilities, maintainer scripts do not erase application state. R7 APPROVE checkpoint honesty: README/WIP explicitly identify scaffold, remaining firstboot/TLS/base-policy/APT/install acceptance and missing release licensing instead of claiming P10 complete. This verdict is for the checkpoint only, never full appliance release or P10 completion.

## Actual independent validation

```text
python3 deploy/debian/vrx/tests/test_packaging.py
Ran 4 tests in 0.005s
OK
bash -n deploy/debian/vrx/prepare.sh
exit 0
source ../toolchain/env.sh; tools/ci.sh check --base origin/main
ok: gitleaks — scanned ~12355 bytes (12.35 KB) in 134ms no leaks found
check PASSED (0m02s)
pnpm --filter @ngfw/api deploy --prod /tmp/vrx-review-deploy-check-20261002
198 packages added; Done in 11.7s using pnpm v12.5.1
```

Production dependency deployment was executed into a reviewer temporary directory without executing appliance services. This proves deploy command compatibility with current pnpm workspace, not that absent compiled dist is bootable. shellcheck was unavailable (`command not found`), so no shellcheck PASS. Existing four static checks do not exercise installed startup tool discovery or missing writable paths and therefore do not resolve the findings.

VPP verification invokes the existing `--require-files --install-gate` validator; version regex requires product `26.06-release+vrx<N>` and exact meta dependency substitution. The checkpoint does not yet publish an APT repository or ship VPP artifacts; ship-only selection belongs to forthcoming publisher review. Dirty checkout rejection protects tracked sources but is not evidence ignored dist outputs match current SHA; unchanged complete gate from the exact checkout remains required before artifact staging.

No actual .deb build, lintian, install/remove/reinstall, unit security score, firstboot, runtime integration or lab acceptance was run. Missing repository license is a documented release gate; reviewer does not invent licensing. Fresh review of subsequent firstboot/TLS/publisher changes is required.
