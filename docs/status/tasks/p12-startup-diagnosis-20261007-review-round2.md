# P12 independent correction review — APPROVE (source scope)

Exact corrected source ac927c6c6, delta from reviewed blocked c7ef8faa0. Applicable R1/R2/R4/R7. Earlier BLOCK report retained verbatim; this is its fix-verification receipt, not deletion of history.

The producer identity guard rejects absent identity, saved namespace different from owned current scope, changed starttime or changed namespace. Guard runs before proc-root inventory, after directory enumeration, before each handle open and after ioctl/fstat before observed/retained acceptance. Open descriptor closes on changed producer through finally. No failed guard widens observed/retained namespace membership. The saved actual process identity is now part of namespace discovery ownership, in addition to NS_GET_NSTYPE, held inode and forbidden host checks. No deadlines or original Wave-B guards weakened.

Actual independent controls in own temporary corrected review checkout:

```
python3 test/topology/frr-linuxcp/private-fib.py --self-test
Ran 11 tests in 0.040s
OK
```

Reviewer reran the original independent reproduction, extended to six negative combinations. For changed-starttime/same-netns and unchanged-starttime/foreign-netns, each before inventory, during inventory and at os.open: PASS; no foreign retained or observed scope. Before/during inventory opens zero descriptors; change at open closes exact fd42 once. These controls are independent of developer fixture and shift the after-open mutation earlier than the developer ioctl fixture. Actual output:

```
PASS independent ('new-start','net:[777]') before/inventory/open no retained/observed foreign scope
PASS independent ('old-start','net:[888]') before/inventory/open no retained/observed foreign scope
```

`git diff --check`: exit0/no output. Corrected temporary merge aborted after verification; no product changes retained in reviewer branch. Go cleanup helpers are unchanged from initial passing focused race check1.343s and not rerun unnecessarily. No live namespaces, mounts, daemons, VPP or new CI; no native mgmtd or200route acceptance claim. Unknown mgmtd30s failure remains deferred.

Verdict: prior producer identity BLOCKER resolved; APPROVE corrected recovered P12 source scope. Operational native acceptance remains open.
