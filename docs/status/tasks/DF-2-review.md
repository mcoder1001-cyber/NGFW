# DF-2 — independent review

Reviewer: review agent (did not write this code). Branch `task/DF-2` @ 45c3da6, base `main` (now at 3a2cf8e: DF-4 and P02b merged).
Ran directly on the host with the slot-3 env (`eval "$(tools/lab env 3)"`, `VRX_INTEGRATION=1`), one package at a time.
`systemctl show vpp -p NRestarts` was `2` before and after every run, so VPP did not restart. `VRX_DF2_PROXY_ND` was not set.

## Checklist

| # | Check | Result |
|---|---|---|
| 1 | Contract | `git diff --name-only main...task/DF-2 -- packages/schema packages/proto apps/agent/gen packages/proto/gen packages/api-client/src/generated` is empty. The values are package-local protos, the D-055 stand-in. OK. |
| 2 | Real verification | All 9 host packages rerun by me: `ip_neighbor`, `arp`, `ip6_nd`, `urpf`, `adl`, `abf`, `classify`, `ip_session_redirect`, `df2/idempotency`. All PASS, with the same Retrieve lines as the pasted output (`TestProxyNdOnHost` is SKIP, on purpose). The tests assert on VPP state via Retrieve, not on in-memory maps. Exceptions: `adl` and 3 classify bindings, whose tests only check that the calls return no error. |
| 3 | Restart safety | No agent-restart simulation is pasted (P05 has no agent yet). My own probe was a temporary test, run and then deleted, not committed. It used fresh descriptor instances and reopened the FileStore from disk, for neighbour, abf.policy, classify table/session and redirect. Retrieve == desired, and Retrieve Meta == Create Meta for all 5 (`{SwIfIndex:16}`, `{ACLIndex:2}`, `{Index:5}`, `{TableIndex:5}` ×2). Neighbour and policy were deleted out of band → absent → re-created. OK for retrievable types. Gaps are in findings 1–4. |
| 4 | binapi provenance | Every message comes from `apps/agent/binapi/{ip_neighbor,arp,ip6_nd,ip6_dad,urpf,adl,abf,acl,classify,ip_session_redirect,interface,ip}`. `binapi/` and `tools/binapi-gen.sh` are untouched. I confirmed that `adl` has no dump and `classify` has no output-ACL / ip-table / l2-table dump. `classify_table_by_interface` covers only the INPUT group (`classify_api.c:563`). **However**, `binapi/feature` has `feature_is_enabled`, which can read back presence for two of the "no dump" types (finding 2). |
| 5 | Shared-host rules | Loopbacks `loop3xx` are tagged `w3:`. The fixture refuses to touch a same-named untagged or foreign loopback. Tables 3000–3999, `10.3/16`, `2001:db8:3::/48`. Cleanup is in `t.Cleanup`. Global singletons (`neighbor-config`, `dad`) are read, set and restored. After my runs: `show classify tables` → none, and no `loop3xx` left. No `pkill` and no daemons. OK. |
| 6 | Security | `grep -rn "vppctl\|exec.Command"` over the 8 packages is empty. No secrets. OK. |
| 7 | Transaction semantics | ABF Update adds new paths before removing old ones, so the policy never becomes empty. Good. Classify Create can orphan a table if `store.Put` fails (finding 8). |
| 8/10 | UI / i18n | n/a. |
| 9 | Scope | `df2/idempotency` (a test) and the shared `df2` helpers are in scope. `apps/agent/go.mod`/`go.sum` are outside the owned files and now **conflict with main** (finding 7). |
| 11 | CI | I ran `tools/ci.sh --base main` in the worktree and got `CI GATE PASSED`, EXIT 0. This matches the pasted output. |

## Findings, ranked by severity

### 1. HIGH — a stale classify Store record claims, and gets deleted, another owner's classify table
`classify/table.go:181-199` (`LiveTables`), used by table/session/input-acl/redirect Retrieve.
A record is trusted if its **index** is still listed by `classify_table_ids`. Classify table indices are not unique over time: VPP reuses them after a restart, or after any delete.
- **Probe (host, w3):** I created a table directly through binapi to stand in for another owner's table. It got index 5. I put `{Name:"w3-stale", Index:5}` into a Store. `TableDescriptor.Retrieve` then returned `classify.table/w3-stale meta={Index:5}` with the foreign table's geometry.
- **Failure scenario:** VPP restarts while the agent is down or before its first Retrieve. On this host that has happened (NRestarts=2, questions #6). Another slot, a DF-7 policer-classify store, or an operator then creates tables that reuse the indices. The agent's persisted FileStore claims them. They are "owned but not desired", or differ from desired, so the reconciler **Deletes or recreates another owner's table**. This breaks the scheduler ownership rule ("two agents … must never touch each other's objects"). In production it can also delete tables created by a second store in the same agent.
- **Fix:**
  - Stamp the Store with the VPP instance identity (`memclnt.ControlPingReply.VpePID`, already in binapi). Drop every record when it differs.
  - Also check each record against `classify_table_info` geometry (skip/match vectors + mask). Drop it on a mismatch instead of reporting it.
  - Add a unit test for both.

### 2. HIGH — the five write-only types are registered by default, and output-acl can silently keep the wrong table
`adl/adl.go:97,188`, `classify/bindings.go:168,267,464`, and `Register` at `classify/bindings.go:469`, `adl/adl.go:192`.
- **Reconciler behaviour:** Retrieve returns `ErrRetrieveUnsupported`. If P05 treats a Retrieve error as fatal (contract step 3: "actual = union of Retrieve"), just *registering* these descriptors fails every transaction. If P05 treats it as "empty actual", then:
  - (a) every transaction plans a Create for them;
  - (b) the verify step (step 5) never matches;
  - (c) no Delete is ever planned, so **removing an output ACL / ADL / ip-table binding from the config never removes it from VPP**, before or after an agent restart. On a firewall that means stale filtering or stale steering.
- **Output ACL, concrete case:** VPP's `vnet_set_in_out_acl_intfc` returns 0 **without changing the table** when the feature is already enabled (`/root/vpp/src/vnet/classify/in_out_acl.c:111-115`). Suppose the desired table changes A→B. With no actual state the planner emits Create, not Update/ErrRecreate. `output_acl_set_interface(is_add, B)` then succeeds and **A stays bound**. Nothing reports it.
- **Readback is partly available:** "no dump" is only partly true. `binapi/feature` has `feature_is_enabled`:
  - `adl.interface` = arc `device-input`, feature `adl-input` (`plugins/adl/adl.c:161`);
  - `classify.output-acl` = `ip4-output`/`ip6-output`, features `ip4-outacl`/`ip6-outacl` (`in_out_acl.c:23-28`).
  Presence readback on owned interfaces would give these types keys (drift, leftover delete, restart re-create). The table names can come from the classify Store the same way memory_size already does.
- **Fix:**
  - Implement presence Retrieve via `feature_is_enabled` for `adl.interface` and `classify.output-acl`.
  - Make `OutputACLDescriptor.Update`/Create unbind first when a binding exists.
  - Keep the three remaining truly write-only types (`adl.allowlist`, `interface-ip-table`, `interface-l2-tables`) **out of the default `Register`**, for example behind an explicit `RegisterWriteOnly` or an option, until P05 answers questions #2. Fix the doc tables to match.

### 3. HIGH — objects on untagged (physical) interfaces are invisible to Retrieve
`df2/ifaces.go:67-82` (`Owned` = owner tag only), used by `neighbor.go:149`, `raconfig.go:253`, `raprefix.go:156`, `proxynd.go:143`, `urpf.go:146`, `arp/interface.go:99`, `abf/attach.go:123`, `classify/bindings.go:370`.
DPDK ports (`GigabitEthernet*`) have no tag, and nothing in main tags them (P05 core not merged). This is where uRPF, RA, static neighbours, ABF attach and input ACLs are really configured.
- **Failure scenario:** on a WAN port, Retrieve never reports these objects. The result is a Create on every transaction, a verify mismatch, and no Delete ever: disabling uRPF or detaching an ABF policy on a physical port never takes effect.
- **Precedent:** DF-4 solved the same problem with a persisted `ClaimStore` for untagged interfaces (`acl/etype.go:26-94`, merged).
- **Fix:** attribute objects on untagged interfaces through a claim store. Reuse DF-4's pattern and share it via `df2`. Alternatively, get a P05 decision that the agent tags physical ports at start-up, and document which one applies.
- **Related:** Create resolves any interface by name without an ownership check (e.g. `ip_neighbor/neighbor.go:90`, `adl/adl.go:30`). On the shared host a mistyped desired object configures another slot's loopback. Refuse foreign-tagged interfaces, as DF-4 does with `ErrForeignInterface`.

### 4. HIGH — `ip6-nd.proxy` is registered for production although it aborted the shared VPP
`ip6_nd/register.go:12`.
The journal for 2026-09-23 15:52:38 shows `os_panic` in `clib_mem_heap_realloc_aligned ← vlib_put_next_frame ← vnet_interface_output_node_fn`. This came right after the first `ip6nd_proxy_add_del` on a loopback (questions #1). That is data-path frame growth, consistent with an ND proxy loop.
- **Problem:** the descriptor is only fake-tested, and one Register call puts it in reach of any desired config.
- **Fix:** leave it out of the default `Register` until the manager reproduces the crash under the exclusive lock (questions #1). Add a Create guard that rejects loopback / non-ethernet interfaces if that turns out to be the trigger. Mark it "unverified on host" in `docs/agent/descriptors/ip6_nd.md`.

### 5. MEDIUM — ABF resolves duplicate-tag ACLs differently from DF-4, which is now merged
`abf/acl.go:24-40`: `byName[name] = d.ACLIndex`, so the last (highest) index wins.
- **Probe (host):** two ACLs tagged `w3:dup` got indices [2, 1]. `abf.DumpACLs` resolves **2**. DF-4 (`acl.LookupIndex`, doc "Duplicate tags") treats the **lowest (1)** as `acl.acl/dup` and reports 2 as `acl.acl/dup#2` for deletion.
- **Failure scenario:** after a lost `acl_add_replace` reply, ABF binds policy X to ACL 2. DF-4 then tries to delete ACL 2, which VPP refuses while ABF holds it. DF-4's transaction then fails on every reconcile, or the policy stays on the non-canonical ACL and diffs as equal.
- **Fix:** rebase on main and use `acl.LookupIndex` for Create. For Retrieve, map index→name with DF-4's rule, reporting `name#idx` for non-canonical indices so the policy is recreated. Drop the local `DumpACLs`. The key string `acl.acl/<name>` already matches DF-4's `acl.KeyACL`.

### 6. MEDIUM — `LiveTables` deletes Store records inside Retrieve, which races with Create
`classify/table.go:192-193`.
Retrieve may run at any time, concurrently with a transaction (scheduler contract). Here is the interleaving:
1. Retrieve reads `classify_table_ids`, which does not yet list the new table.
2. `TableDescriptor.Create` adds the table and `store.Put`s it.
3. Retrieve's `store.All()` sees the new record, finds its index missing from its own snapshot, and **deletes the record**.

The table is then orphaned in VPP and invisible. The next reconcile Creates a second one, and the first one leaks forever.
- **Fix:** make Retrieve side-effect free, filtering only. Prune stale records only in a Create/Delete path, or under a mutex shared with Create. At minimum, take `store.All()` before `classify_table_ids`.

### 7. LOW — branch no longer merges cleanly; go.mod edited outside the owned files
`git merge-tree main task/DF-2` → CONFLICT in `apps/agent/go.mod` and `go.sum` (indirect `fsnotify`/`logrus` for the govpp socketclient, which DF-4 also added).
- **Fix:** rebase on main (this is also needed for finding 5) and keep main's go.mod lines. The change is not disclosed in DF-2.md; mention it.

### 8. LOW — smaller correctness items
- `classify/table.go:140`: the VPP table is created, then `store.Put` fails → an orphaned untracked table. On a Put error, delete the table just created before returning.
- `classify/session.go:172`: a desired `classify.session` with the same match as an `ip-session-redirect` in the same table is excluded from Retrieve. Its Create silently overwrites the redirect's session. Reject that combination in validation, or document it.
- `ip6_nd/dad.go:89`, `ip_neighbor/config.go:80`: these global singletons have no owner. On the shared host, any slot whose reconciler registers them deletes or overrides another slot's setting. That is fine in production (one agent). Note it in the doc and in `shared-host-rules.md` via the manager.

### 9. INFO
- Evidence reproduced exactly: per-package host runs, the idempotency plan (13 creates → empty), and CI. The pasted vppctl output is consistent with my runs (table index differs: 5 vs 0, as expected).
- The idempotency test reuses the same descriptor instances for both applies. Add a fresh-instance / reopened-Store pass, as in my probe, so restart safety is covered by the committed test.
- The classify.session vs ip_session_redirect exclusion is correct and restart-safe. It comes from `ip_session_redirect_dump`, not the Store, and handles both the skip-offset and the full match layout.
- ID-range attribution for proxy-ARP and ABF: `proxy_arp_dump` returns the **table id**, not the fib index (`arp_api.c:79-80`), so range filtering is sound. ABF additionally requires the policy's ACL to carry this owner's tag. On the shared host neither can claim another slot's objects. With the production `nil` range the agent owns every proxy-ARP range, which is intended for a single agent. State that in `arp.md`.

## What unblocks

Findings 1, 2, 3 and 4 must be fixed (or 2/3 settled by a P05 decision recorded in the LOG). Finding 6 must be fixed, and the branch rebased (findings 5 and 7). Findings 8 and 9 can follow.

**BLOCK**
