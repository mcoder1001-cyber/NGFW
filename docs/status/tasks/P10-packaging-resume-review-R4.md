# P10 resume — independent R4 daemon/privilege review

Reviewed frozen product `db721ff49a747f8e7453ad1b4f4e11d085237d95` plus evidence `ee8c7ef9`; manager reports remote PR63 `ec5d0ae0` has the same tree. Review includes hardening `aa76368a`. Reviewer read R4/shared review instructions, shared-host rules, pending host handover, P10 prior review/arbitration reports and pending ownership decision. No product edits or host operations.

## Findings

**MAJOR — deploy/systemd/vrx-agent.service:12–13: clean appliance agent cannot start.** Unit sets globals ownership but neither `VRX_VPP_ID_RANGE` nor `VRX_VPP_TABLE_BASE`. Optional `/etc/vrx/agent.env` is neither provisioned nor documented by P10. `agent.ConfigFromEnv` calls `ResolveIDScope`; `Config.Validate` returns its ErrNoIDRange, and main exits before connecting to VPP. This is a deterministic configuration defect, not laboratory uncertainty. Fix the appliance-only unit to supply `VRX_VPP_ID_RANGE=all` as already specified by shared-host-rules §12, with regression coverage and a never-shared-host warning. This existing authorized appliance ID contract does not authorize changing CAP_CHOWN or `/etc` write boundaries.

Independent source inspection at frozen head:

```text
Environment=VRX_GLOBALS_OWNER=1 VRX_SOCKET_GROUP=vrx VRX_AGENT_STATE_DIR=/var/lib/vrx/agent
EnvironmentFile=-/etc/vrx/agent.env
packaged ID scope present: False
provisioning/docs ID scope references: 0
```

**Initial R4 verdict: BLOCK** until the startup configuration finding is corrected. Existing R8 bounded storage review does not override this newly identified R4 failure.

## Remaining reviewed boundaries

No VPP generated binding/API/C changes; no new VPP object or reconcile deletion semantics. Storage fixture redirects host paths and account operations. Installer fixture replaces package/service operations privately and retains no-start policy handling. Unit hardening retains agent netlink/network and required non-cgroup namespaces; no broad filesystem/capability expansion. Review invokes no live VPP, daemon, privileged network operation, host package install or restart. Product-only global ownership settings must never be exercised on the shared host while handover remains pending.

CAP_CHOWN/daemon-file ownership and atomic system-identity parent writes remain real parked product decisions. Static initial firewall is not dynamic punt admission. Approval of a corrected bounded checkpoint may allow integration as explicitly incomplete P10, but cannot certify installed daemon operability or resolve these pending decisions. Real appliance boot, reconciliation/restart and packet evidence remain NOT RUN; owner laboratory deferral does not convert unfinished code or privilege work to PASS.

## Bounded verification — d31af805376609c7587f862067caa4296d6dde58

Original MAJOR resolved. Appliance unit now explicitly supplies `VRX_VPP_ID_RANGE=all`; no TABLE_BASE is added. Unit comment and installation guide explicitly forbid starting this appliance unit on the shared lab host and warn against contradictory variables. This implements the pre-existing shared-host-rules §12 appliance contract, not a new privilege grant. CapabilityBoundingSet and filesystem restrictions remain unchanged. Offline packaging regression now checks explicit all-ID scope and absence of a conflicting table base.

Independent execution in own detached verification worktree at exactly `d31af805`:

```text
python3 deploy/debian/vrx/tests/test_packaging.py
Ran 9 tests in 2.018s
OK
git diff --check ee8c7ef9 HEAD
(no output; exit 0)
```

Additional pre-fix attempt to execute existing agent `TestConfigFromEnvIDRange` could not reach tests: missing module downloads failed with `socket: operation not permitted`; setup failed, NOT PASS. No test assertion failed. Reviewed the existing test and actual ConfigFromEnv/Validate/ResolveIDScope implementation: the supplied `all` configuration is the explicitly accepted appliance case; absence produces ErrNoIDRange. Complete exact-head quick remains a separate mandatory gate.

**Final R4 verdict: APPROVE bounded corrected packaging checkpoint at d31af805.** Initial BLOCK is superseded only for the corrected ID-scope finding. No new unresolved R4 finding in this delta. This permits code integration as explicitly incomplete P10 after remaining gates; it does not certify installed operability, resolve the ownership/filesystem PENDING, authorize shared-host execution, or waive unfinished product work.
