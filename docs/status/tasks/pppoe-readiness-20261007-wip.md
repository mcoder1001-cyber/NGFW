# PPPoE readiness WIP

Branch/worktree: codex/pppoe-readiness-20261007, /root/ngfw-wt/pppoe-readiness-20261007.
Initial published checkpoint eede5925f: CLI push succeeded. Containing checkpoint
SHA: `git rev-parse HEAD`; remote: `git ls-remote origin refs/heads/codex/pppoe-readiness-20261007`.
Owned files: task envelope, PPPoE renderer README, PPPoE dependency hunk in Debian
control and PPPoE-only dependency regression in deploy/debian/ngfw/tests/test_packaging.py.

Completed code: ngfw-agent now directly depends on ppp and pppoe, retaining
python3/dhcpcd-base. Standalone agent installation no longer omits PPPoE tooling.
README documents prerequisites separately from unsupported forwarding behavior.
Actual focused verification: `TMPDIR=/ppr python3 -B deploy/debian/ngfw/tests/test_packaging.py Packaging.test_pppoe_runtime_dependencies_belong_to_agent Packaging.test_runtime_dependency_contract -v` PASS, 2 tests, 0.001s. `git diff --check` clean.
No aggregate CI (owner waiver), packages installed, services restarted, packets sent,
VPP/plugin/binapi edits or live acceptance performed.

Recovery inventory: origin/codex/design-pppoe-client-datapath-20261007 at a0134e49d
contains design only; its PENDING decision lists native single-WAN40–64 agent-hours,
multi-WAN96–160, kernel routed hop64–104. Each requires architecture/C/security
approval; no currently authorized contract supplies a source-only product fallback.
Historical native checkpoint4d0260dc7 depended on private VPP C safety patches and
single-client support; it is not a safe drop-in solution under current envelope.
Current main PPPoE lifecycle source equals origin/codex/resume-pppoe-20261007
for renderer and runtime paths. Arbiter ruling81be12f2a remains mandatory: transition
admission after stop can resurrect an old writer before invalidation/replacement;
parent pppd liveness checks numeric PID without pinning original identity.
These are actual product failures, not laboratory-only acceptance deferrals.

Remaining: independent packaging review/integration; separately repair lifecycle
transition admission and parent identity with applicable independent closure.
Product discovery and IPv4/IPv6 LAN encapsulation still unsupported. PD assignment
to LAN and live dial/reconnect/rollback also remain unresolved. No DONE claim.
Current failure: above lifecycle and datapath findings; focused packaging tests pass.
Exact next command: `git push origin codex/pppoe-readiness-20261007`, then create a
small draft packaging PR and attach it. Root coordinates lifecycle repair scope.
