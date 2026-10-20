# Task envelope — host 172.30.110.211 forwards too little traffic (VPP resources / config)

Owner request (2026-10-20, Persian): "Why does machine 110.211 not pass more traffic? Can't you give
VPP more resources or a better configuration?"

Base: origin/main 921f82388. Branch: claude/machine-110-211-traffic-8c1edc (worktree
`/root/NGFW/.claude/worktrees/machine-110-211-traffic-8c1edc`).

Owned files: `docs/status/tasks/machine-211-vpp-tuning*`,
`apps/agent/internal/renderers/vppstartup/{model.go,render_test.go}`, `docs/agent/renderers/vppstartup.md`.
Host 172.30.110.211: read-only diagnosis plus **runtime-only** VPP CLI changes (`set interface
rx-placement`, `set interface tx-queue`) that a VPP restart undoes; no file on the host was written,
`/etc/vpp/startup.conf` untouched, no VPP restart, no port bounce.

Acceptance: root cause of the packet loss named with evidence from the host; loss removed at runtime
with measured counters; a reviewed permanent configuration (dataplane document + rendered
startup.conf + sha256 for `deploy/vpp/apply-startup.sh`) prepared for the manager apply step; the
product warns about the misconfiguration in future renderings. No throughput claim beyond what the
owner's generator offered during the measurement.
