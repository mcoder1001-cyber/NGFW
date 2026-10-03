# P10 final activation/layout recheck — R2/R4/R8

Exact frozen head `00cb2cd33fde9a60ba1cf5ccd8b384947b72bbd1`, isolated task/P10-punt-final-review. Compared with provider snapshot412f4d30 and approved Element helper scope.

**APPROVE this final provider/layout delta.** No new MAJOR/BLOCKER identified. Complete P10 appliance acceptance and unresolved privilege decisions remain separate.

- Bootstrap retains permanent `punt_interfaces`, introduces EMPTY `dynamic_punt_interfaces` and a distinct accept rule. Management22/443 and IPv6-neighbor rules remain separate; no wildcard or ruleset/table mutation from agent. Element argv/parser identity now exclusively targets `inet ngfw_base dynamic_punt_interfaces`; static set is never read or written through the dynamic mutation path. Overlap with permanent configuration is rejected, rather than becoming deletable shared ownership.
- Descriptor initialization clones permanent inputs into runtime bound enforcement; Element validates combined permanent+dynamic union64 before mutation. Projection and descriptor final-view validator preserve the same union cap and management exclusion. Config/renderer initialization happens before concurrent usage; no mutable external slice is retained.
- Projection uses explicitly supplied validated desired DefaultNetns KV before querying current default; malformed supplied value produces failure, not root inference. Optional DefaultNetns dependency adds lifecycle ordering for future transactions without requiring a global key when absent. Existing actual owned pair provider continues to use effective VPP readback namespace; it does not reinterpret historical pairs using today's default.
- Existing explicit NGFW_BASE_POLICY/ngfw/globals-owner activation, trusted bounded root config, fixed nft runner, readback limits, compensation context and typed uncertainty remain unchanged. No new capabilities, writable directories, broad management admission or live host calls occurred.

Personally executed on exact frozen head using persistent Go caches/workspace binary:

```
go test -race -count=5 ./internal/renderers/basepolicy
ok ngfw/agent/internal/renderers/basepolicy 1.150s

go test -race -count=5 ./internal/subsystems -run '^TestBasePolicy'
ok ngfw/agent/internal/subsystems 1.062s

python3 -m unittest discover -s deploy/debian/ngfw/tests -p 'test_base_policy.py'
Ran 3 tests in 0.002s
OK
```

The basepolicy result came from a command also launching all subsystem tests; that unrelated broad subsystem repetition completed with FAIL after72.858s before the attempted exact-PID termination (both PIDs already gone). Its large output was truncated before failure identity was available. Broad subsystem result is FAIL/unclassified and must not be reported as passing; manager must inspect hosted complete gate before merge. Focused tests independently cover final desired root/non-root override without stale getter, disabled/nonproduct activation, combined runtime cap, helper/parser failure cases and scheduler membership fixtures. Python tests cover empty dynamic bootstrap, safe explicit static inputs and boot-order dependencies. Installed systemd/nft/VPP traffic/restart acceptance: NOT RUN. Host CAP_CHOWN/global /etc atomic-write decisions remain untouched.
