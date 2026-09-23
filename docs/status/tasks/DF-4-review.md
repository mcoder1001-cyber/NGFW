# DF-4 — review (independent reviewer)

Branch `task/DF-4` @ `14defa2` (base `030f404`; main is now `a2ed2e9`, 42 commits ahead — `git merge-tree` against main is clean).
Reviewed: `apps/agent/internal/descriptors/acl/**`, `docs/agent/descriptors/acl.md`, `docs/status/tasks/DF-4*.md`, the `go.mod`/`go.sum` delta.

## What I ran (on the host, slot 10)

| Check | Result |
|---|---|
| `tools/ci.sh --base main` in the worktree | **CI GATE PASSED** (1m20s). Matches the output pasted in `DF-4.md`. |
| `VRX_INTEGRATION=1 VRX_TEST_PREFIX=w10 VRX_ACL_STATS_RESTORE_DISABLED=1 go test -race -count=1 -run TestACLPluginOnHost -v ./internal/descriptors/acl/` | **PASS**, all 8 subtests (acl, acl-50-rules, interface-binding, etype-whitelist, macip, stats, delete, macip-del-unbinds). Every Retrieve check is followed by an empty re-apply plan. |
| Host state before/after | `show acl-plugin tables`: "Stats counters enabled for interface ACLs: 0" before and after; 0 `w10` ACLs, 0 `w10` MACIP ACLs, 0 `loop104x` after. |
| Extra probe (my own temporary test, run and then deleted; not committed): update a **bound** ACL in place, update a **bound** MACIP ACL in place, simulate an agent restart with **fresh descriptor instances**, delete a binding directly through binapi, then re-Create it | All PASS. Fresh-instance Retrieve gives back the same Meta that Create returned, for all 5 VPP object types: `{ACLIndex}`, `{ACLIndex}`, `{SwIfIndex}`, `{SwIfIndex}`, `{SwIfIndex, ACLIndex}`. Bound ACL/MACIP updates keep their bindings, and Retrieve == desired. The binding lost from VPP disappears from Retrieve and is re-created. Findings 2 and 4 below come from this probe. |
| VPP bug V7 | Confirmed in `/root/vpp` (tag v26.06), `src/plugins/acl/acl.c:1826`: `REPLY_MACRO (VL_API_ACL_DEL_REPLY)` in `vl_api_acl_stats_intf_counters_enable_t_handler`. The raw-stream workaround is justified. |

## Checklist

1. **Contract**: no hits in `packages/schema`, `packages/proto`, `apps/agent/gen`, `packages/api-client`. OK.
2. **Real verification**: the integration test talks to `/run/vpp/api.sock` and `/run/vpp/stats.sock`. Its assertions use descriptor `Retrieve` (decoded from `acl_dump`, `acl_interface_list_dump`, `macip_*_dump` and `acl_interface_etype_whitelist_dump`) plus a stats-segment read. The `vppctl show acl-plugin …` output in the status file is from an operator-side capture, and it matches the objects I saw. This proves real VPP state, not mocks. OK.
3. **Retrieve / restart safety**: Retrieve exists for every VPP object type. The stats reader is Retrieve-only by design. My fresh-instance probe confirms that Meta is rebuilt exactly. The factory template does not require a running-agent restart. One real gap remains: finding 1 (stats-enable across a VPP restart).
4. **binapi provenance**: every message comes from the generated `binapi/acl`, `acl_types`, `ip_types`, `ethernet_types` and `interface` packages, and it compiles. The branch does not touch `apps/agent/binapi/` or `tools/binapi-gen.sh`. OK.
5. **Shared host**: tags are `w10:<name>` (via `vpp.OwnerTag`). The loopbacks are slot-10 instances (`loop1040/1041`). `vpptest.LockLab` takes the shared lock. Cleanup is in `t.Cleanup` and runs in LIFO order (unbind, then ACL delete, then loopback delete, then optional flag restore). No `pkill`, `killall`, `exec.Command` or `vppctl` in the package, and `local0` is never touched. One exception: finding 5 (the global counters flag is left on by default).
6. **Security**: no shell calls, no secrets, no routes. OK.
7. **Transaction semantics**: every mutation is a single atomic VPP call (`acl_add_replace`, `acl_interface_set_acl_list`, `…_set_etype_whitelist`, `macip_acl_interface_add_del`). Update of an ACL or MACIP ACL keeps its index, so rollback by "Update back" is sound. OK.
8. **UI** / 10. **i18n**: not applicable.
9. **Scope creep**: none. `LookupIndex` and `GetPluginInfo` are asked for in the prompt. The `go.mod` edit is outside the owned files (finding 7).
11. **Tests actually run**: I reproduced them (above).

## Findings (by severity)

### 1. MEDIUM — `acl.stats-enable` Retrieve goes stale after a VPP restart, so the counters stay off silently
`apps/agent/internal/descriptors/acl/stats_enable.go:99` and `:124-131`.
Retrieve returns `d.applied`, a value kept in process memory. Nothing clears it when VPP restarts.
- **Failure scenario:** the agent enables the counters. Later `tools/lab restart-vpp vrx-a` or a `kill -9 vpp` happens after handover. The agent reconnects, but Retrieve still reports `{enabled:true}`. The diff is empty, so the counters are never re-enabled. `StatsReader` then returns zeros forever. This breaks DoD #2 ("survives restart-vpp, agent reconciles") for this object.
- **Fix:** tie `applied` to a VPP identity. Options:
  - Store the VPP boot time (stats `/sys/boottime`) or the client's connection generation next to `applied`, and return nothing from Retrieve when it changed.
  - Or expose a `Reset()` hook that P05 calls on reconnect.

  Add a unit test with the fake for the chosen approach.
- **Related (LOW):** with `enabled:false` desired, Retrieve reports `false` even when the flag is on in VPP, so Retrieve does not match VPP. This is documented. Keep it, but the doc should say plainly that `false` means "not managed", not "off".

### 2. MEDIUM — ethertype whitelists on untagged interfaces never converge and leak
`apps/agent/internal/descriptors/acl/etype.go:151`.
Retrieve keeps a whitelist only when the *interface* tag belongs to this owner. Physical/DPDK ports are normally untagged, and ethertype whitelists are mostly used on them.
- **Failure scenario** (reproduced with an untagged slot loopback `loop1043`): Create applies the whitelist, but Retrieve never shows it.
  - Every reconcile plans a Create, so the plan is never empty (breaks the idempotency acceptance).
  - The verify step after apply ("re-Retrieve and compare") fails.
  - When the whitelist is removed from desired state, the scheduler never Deletes it. It stays on VPP forever.

  The doc caveat mentions this, but the consequences above are not stated, and F-* will hit it on day one.
- **Fix** (pick one and document it):
  - (a) Make the tag a precondition: `Create` returns a clear error when the interface is not tagged by this owner, so nothing is applied half-invisibly. DF-1/P05 then tags physical ports.
  - (b) Fall back to an owner registry (the owner-table mechanism mentioned in `scheduler/descriptor.go` "Ownership") for untagged interfaces.

  Also add a question for DF-1/P05 about tagging physical ports.

### 3. LOW — empty desired binding or whitelist passes Validate, so the plan never empties
`spec.go:546` (`InterfaceBinding.Validate`) and `spec.go:573` (`EtypeWhitelist.Validate`), together with `binding.go:185` and `etype.go:144`, which skip empty lists in Retrieve.
- **Failure scenario:** a desired `{"interface":"x","input":[],"output":[]}` (the doc even says "both empty = unbound") is Created on every reconcile. Verify-after-apply fails, because Retrieve never reports it.
- **Fix:** have `Validate` reject a binding or whitelist with both lists empty ("omit the object to unbind"), and add a unit test. Update the doc line "both empty = unbound".

### 4. LOW — duplicate owner tags produce duplicate keys; the orphan is never cleaned up
`acl.go:172` and `macip.go:160`.
Two VPP ACLs can carry the same `w<N>:<name>` tag, for example when the `acl_add_replace` reply is lost or times out, the transaction rolls back, and then retries Create. In that case Retrieve emits two KVs with the same key, and `aclIndexByName` silently picks the last one.
- **Consequences:** the scheduler's behaviour with duplicate actual keys is undefined. One ACL is an orphan that is never deleted. A binding can end up on a different index than the one Meta holds.
- **Fix:** in `dumpOwnedACLs` / `dumpOwnedMacipACLs`, keep the lowest index per name. Either delete the extras or report them under a distinct, owned-but-not-desired key (e.g. `acl.acl/<name>#<index>`) so the scheduler deletes them. Add a unit test with the fake.

### 5. LOW — the integration test leaves the global counters flag on by default
`integration_test.go:182`.
The factory prompt says "global singleton; read-modify-restore in tests". By default the test enables the flag and leaves it on. It is restored only when `VRX_ACL_STATS_RESTORE_DISABLED=1` is set, and `tools/ci.sh full` will not set that. On the shared host this changes global VPP state for every other slot (the data plane costs more once counters are on).
- **Fix:** reverse the default. Restore to disabled unless `VRX_ACL_STATS_KEEP=1` is set. The flag was 0 before every run so far, and nothing on main enables it. Alternatively, get a manager decision recorded in `shared-host-rules.md`. (Question 6 is open, so the manager should settle it before merge.)

### 6. LOW — interface-binding ownership: a foreign ACL in the list makes the whole binding invisible, and Create then overwrites it
`binding.go:197`.
If another owner has bound its ACL on the same interface, Retrieve skips the binding. Our Create then sends `acl_interface_set_acl_list` with only our ACLs, which silently unbinds the foreign one. This contradicts the "two owners never touch each other's objects" rule in `scheduler/descriptor.go`. It is unlikely in production (one owner) but possible on the shared test host.
- **Fix:** in `setList`, dump the current list first. Refuse with a clear error, or preserve the foreign entries, when the list contains ACLs that are not ours. Document which of the two behaviours was chosen.

### 7. INFO — process / documentation
- `apps/agent/go.mod` and `go.sum` gain three `// indirect` modules. This is outside the owned file set, but it was disclosed (Q7), the merge against current main is clean, and the modules are needed by the govpp socket/stats adapters. Acceptable. The manager should merge DF-4 before P05 touches `go.mod` again, or rebase.
- `docs/agent/descriptors/acl.md:29` says "The P03 proto contract has no ACL messages yet". Main now has `AclConfig`, `AclList`, `AclRule`, `MacipList` and the attachment messages (domain level). The leaf messages are still P03b's (D-055), so the structpb stand-in is still allowed. Reword the sentence to "no *leaf* ACL messages yet".
- The key spelling is `acl.acl/<name>` rather than the prompt's `acl/<name>`. This follows the frozen scheduler contract. Tag spelling is `<owner>:<name>` via `vpp.OwnerTag`. Both are sound. The manager should log D-DF4-2 and D-DF4-3 and point DF-2 at `acl.KeyACL` / `acl.LookupIndex`.
- `StatsReader.ReadOwned` runs one `acl_dump` plus one regex `DumpStats` per ACL on every call, and it is meant to run at 1 Hz. Performance is out of scope, but a single `DumpStats(StatsPathPattern)` per tick would be a trivial improvement later.
- Nothing on the host tests a bound-ACL update. My probe shows it works (VPP re-applies the tables). Consider adding it to `TestACLPluginOnHost`, because F-* depends on it.

## Verdict
All of DF-4's own acceptance items are met, and the evidence was reproduced on the host. Findings 1 and 2 are correctness gaps in restart and convergence behaviour and should be fixed before F-* wiring. Findings 3–6 are small.

**APPROVE WITH CHANGES**
