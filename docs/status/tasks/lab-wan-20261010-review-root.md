# Independent review of manager test portability and restart fix

Verdict APPROVE. Read-only review of root workspace /root/.codex/worktrees/f693/NGFW exact uncommitted diff in apps/agent/internal/agent/rpc_wireguard_integration_test.go and rpc_host_nics_integration_test.go on 2026-10-10.

WireGuard fixture now stops Service, cancels connection work and calls Wiring.Close before recreating the same owner. Real production Agent.Stop already closes wiring. Wiring.Close deletes its closer-map entry before callback invocation; existing t.Cleanup(w.Close) is safely idempotent. The fix repairs leaked owner registration without removing restart assertions or changing product code.

HostNIC exact seven-interface assertion now requires ens192 to carry the real reference management address172.30.126.195 and the previous PCI match. Other VMware guests may share PCI positions. Per-interface nonempty PCI/no DPDK binding, management/data minimums and unchanged VPP restart count/startup configuration assertions still execute for250. No security boundary or product mutation.

No executable edits by reviewer. Manager owns full gate and actual reruns.

RA isolated harness optional compiled exact-test binary review APPROVE. Added required exact === RUN and --- PASS name boundary checks reviewed APPROVE: zero-test exit0 becomes failure, actual subprocess failure preserved, same test/count/timeout and isolation retained. No reviewer edits to root files.
