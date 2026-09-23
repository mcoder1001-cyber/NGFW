# DF-4 — questions for the manager (none blocking; decided and continued per 00-CONTEXT)

1. **VPP 26.06 bug — `acl_stats_intf_counters_enable` replies with the `acl_del_reply` message id**
   (`src/plugins/acl/acl.c`, handler ends with `REPLY_MACRO (VL_API_ACL_DEL_REPLY)`; verified against the v26.06 tag and
   on the host: the generated `Invoke` would reject the mismatched id). Configuration-only fallback in place:
   `acl.EnableCounters` sends the request on a raw `NewStream` and accepts either reply type (works on the host).
   Please add a row to `docs/vpp-code-track.md` (not a file DF-4 owns): one-line C fix upstream / in our build later.
   Nothing to regenerate in binapi — the message definitions are correct, only the C handler is wrong.
2. **Desired-state type.** `packages/proto` has no ACL messages (only `Health`), so the descriptors use `*structpb.Struct`
   documents built from typed Go specs (`spec.go`), exactly as the P05a example does. When P03 adds ACL protos, swapping
   is confined to `spec.go` (+ `docs/agent/descriptors/acl.md`). Who owns adding the proto — P03 follow-up or DF-4?
3. **Interface key scheme.** DF-4.md says the interface key is `interface/<name>`, the descriptors README says
   `interface.loopback/<name>` (DF-1 not merged). The binding descriptors take `acl.WithInterfaceKey(func(name) Key)`
   (default `interface/<name>`) and declare the interface dependency **Optional** so a wrong default cannot fail a
   transaction; the ACL dependencies are mandatory as specified. P05 should wire the real scheme when DF-1 lands.
4. **Key spelling.** DF-4.md publishes `acl/<name>` / `macip-acl/<name>`; the frozen scheduler contract needs the
   descriptor name as first segment and the README names the descriptor `acl.acl` → keys are `acl.acl/<name>` and
   `acl.macip-acl/<name>` (helpers `acl.KeyACL`, `acl.KeyMacipACL`). DF-2 should use the helpers, never the literal.
5. **Tag format.** DF-4.md says tag `w<N>-<name>`; the README/D-030 owner stamp is `<owner>:<id>` via `vpp.OwnerTag`.
   Used the shared helper (`w10:t-lan-in`) so one mechanism serves interfaces and ACLs.
6. **Counters flag restore.** The flag is global and has no read API (only `show acl-plugin tables`). The descriptor
   never disables it; the integration test restores it to *disabled* only with `VRX_ACL_STATS_RESTORE_DISABLED=1`
   after the operator has read the flag (it was `0` before this task's runs and is `0` again after). Confirm this is
   the intended reading of "read-first, never disable if foreign-enabled, restore in Cleanup".
7. **go.mod/go.sum** gained govpp's transitive modules (`fsnotify`, `ftrvxmtrx/fd`, `logrus`, all `// indirect`) via
   `go mod tidy`, needed by `adapter/socketclient` + `adapter/statsclient` in the integration test. P05 needs the same
   modules for the real client; no new direct dependency.
