# Private OSPF FIB acceptance

Branch `codex/closeout-ospf-fib`, base `6619ae6418f3fee5c81e6a546900a6079ced67e6`. Source checkpoint published by manager connector on `codex/closeout-ospf-fib`: remote `e5f87de8dddf054ba6c697ac66c93f33e07c08c3`, local source `06d4b94fdb177f8b9111312aef7e216b9cf12ab9`. Evidence checkpoint below requires publication. Own driver, rootFRR daemon launcher in the existing topology test, and `closeout-ospf-fib*` documents; no product code changed.

The driver unshares network and mount namespaces, verifies the network namespace inode differs from the shared host, permits only initial loopback and kernel-created IPv6 fallback tunnel, and binds private directories over /run/netns and /run/frr. It then invokes existing isolated-vpp.py with the unchanged OSPF root-mode test. FRR and linux-nl share the private namespace's root network view; shared host routes/NICs are absent. Shared VPP PID/restarts are verified unchanged across the run. Existing lab/global locks and Full/100-route/restart/withdrawal assertions remain enabled.

Initial boundary preflight correctly rejected kernel-created ip6tnl0 as unexpected; the explicit initial allowlist now includes only lo/ip6tnl0. Neither connects to the host. First acceptance run revealed a pre-existing rootFRR fixture launcher hang: daemonized FRR inherited the pipe used by CombinedOutput, preventing launcher completion. The manager authorized a narrowly scoped fixture change to capture stdout/stderr in a regular slot-owned file, retaining 30-second context, diagnostic error output, PID ownership checks and teardown. Only owned private-namespace test/mgmtd processes were stopped before rerun. Actual rerun **PASS**, `TestOSPFTopologyOnHost` 59.16 seconds, exit 0. Unchanged strict assertions prove FRR RIB and VPP `lcp-rt-dynamic` both 100 routes; peer withdrawal to 50 and reannouncement to 100 before and after agent restart; restart/pair-loss recovery 7.6 seconds (target ≤30 seconds); rollback both 0; cleanup no owned dynamic routes. Evidence `closeout-ospf-fib-evidence/root-private.log`. Static check PASS, `closeout-ospf-fib-evidence/check.log`. Manager independently reviewed and approved private boundary/regular-file launcher scope before successful rerun.

Exact recovery/running command:

```
env $(tools/lab env 6 | sed 's/^export //') TMPDIR="$PWD/.scratch/go-tmp" GOTMPDIR="$PWD/.scratch/go-tmp" tools/heavy.sh python3 test/topology/ospf/private-fib.py
```

Successful output `.scratch/ospf-private-fib-fixed.log`, copied into committed evidence. The owned disposable VPP stopped normally; shared MainPID1014/NRestarts0 verified unchanged. No shared VPP restart or root-host FRR daemon occurred. This proves live private-namespace AF_PACKET forwarding/control-plane integration; it does not claim physical NIC/DPDK acceptance. Complete quick on the final integration tree remains required.
