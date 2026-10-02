# Multi-WAN monitor independent R8 review

Reviewed `f5962281289941ffd2c0b7a1c30e0165740e3436`, tree `0e7dbc5af6a287e13607a69a544b9ced494615bb`, against main `c76774e8d2e04abee8ea7a301e64acbb3f794373`. Parent associates product with PR65 remote `12bd2f2a`; remote identity not independently verified here. Isolated branch `review/wan-r8`, worktree `/workspace/scratch/de92de7d9874/wan-r8`. Reviewer owns review reports only.

## Finding

**BLOCKER — packaged service cannot run the advertised raw ICMP monitor.** `apps/agent/internal/multiwan/probe_linux.go:135-137` dials `ip4:icmp`, which opens a raw ICMP socket requiring CAP_NET_RAW. The shipping `deploy/systemd/vrx-agent.service:34` bounds capabilities to `CAP_NET_ADMIN CAP_SYS_ADMIN CAP_IPC_LOCK`, excluding CAP_NET_RAW even though the service runs as root. Every ICMP check therefore fails in the installed service and is reported as ordinary packet loss/down. Tests inject a net.Pipe dialer and cannot detect this packaging incompatibility. This is an actual supported-runtime defect, not a deferred lab acceptance item.

Fix within the established security boundary, or explicitly reject/report unsupported ICMP capability rather than claiming healthy implementation. A datagram/ping-socket implementation can avoid CAP_NET_RAW but must account for ping_group_range permissions and kernel-assigned ICMP identifiers; merely changing the network string is insufficient. Expanding service capabilities changes a documented privilege boundary and requires the decision-policy process; this reviewer has not authorized or made that change. Until fixed, HTTP/DNS scope can remain reviewable with an explicit ICMP limitation instead of silent fabricated WAN loss.

## Evidence actually executed

Read service bounding set and production raw dial path above. In a child process without CAP_NET_RAW, opened and closed unconnected sockets only (no packets, no system/daemon changes):

```text
capsh --drop=cap_net_raw -- -c 'python3 <socket-construction-and-device-bind-check>'
raw ICMP: errno=1 Operation not permitted
datagram ICMP: errno=13 Permission denied
TCP device bind: available
```

The Python check used `socket(AF_INET, SOCK_RAW, IPPROTO_ICMP)`, `socket(AF_INET, SOCK_DGRAM, IPPROTO_ICMP)`, and `socket(AF_INET, SOCK_STREAM)` followed by `SO_BINDTODEVICE=lo`; each successful socket was immediately closed. Executed with authorized sandbox escalation solely for socket operations. Environment capability set was empty, so this is not claimed as an exact systemd sandbox recreation; the known missing raw capability is the relevant condition. `/proc/sys/net/ipv4/ping_group_range` here is `65534 0`, explaining why blindly replacing raw ICMP with a ping socket is not sufficient. Initial TCP device binding succeeds here without CAP_NET_RAW; no unsupported claim is made for HTTP/DNS binding.

## Other reviewed failure modes

- Runtime caps total logical probes at 2048, uses per-probe contexts/deadlines, serializes replacement and drains old generation before starting new work; late results cannot become current health. HTTP response headers are bounded; body not consumed; redirect not followed. DNS/ICMP close pending reads on cancellation.
- RPC checks owner and exposes unavailable runtime; observations are independent copies and active route remains empty. Replacement resets observations when interface identity changes; identical configuration preserves hysteresis.
- Persisted monitor snapshot changes only after authoritative save, including mirror-failure distinction, and initializes from saved state on restart. Existing R1 durability and R3 aggregate timestamp fixes were inspected; no duplicate full gate or broad test suite run.
- Missing device/namespace/VRF fail closed. Their failure is currently visible as down/loss, not a specific unsupported reason; observability improvement is a follow-up, separate from the shipping capability blocker above.
- No new dependency, package, systemd mutation, migration, route installation or daemon launch is introduced. Existing unsupported forwarding warning plus user-doc status make monitor-only scope explicit. Live binding, routing, VPP and reboot acceptance remain unexecuted and are not counted as code blockers.

Verdict: **BLOCK** until raw-ICMP packaged-runtime incompatibility is resolved or explicitly bounded out without misleading link-health reports.
