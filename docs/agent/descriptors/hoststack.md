# descriptors/hoststack (F-host-stack)

| Descriptor | Key | VPP messages (apps/agent/binapi) | Retrieve | Notes |
|---|---|---|---|---|
| `hoststack.session` | `hoststack.session/global` | `session_enable_disable_v2` (`RT_BACKEND_ENGINE_API_RULE_TABLE`), probe `session_rules_v2_dump` | write-only | D-071: the globals owner (`RegisterGlobals`) enables the layer only when the probe fails. A non-owner (`Register`) requires it through `Globals.Require` with the probe as the getter. Delete is a no-op: the layer is never disabled or re-engined. |
| `hoststack.namespace` | `hoststack.namespace/<id>` | `app_namespace_add_del_v4` | write-only (no dump) | The re-add is idempotent (VPP updates in place). The reply's `appns_index` is recorded in `Wiring.BootStore()` under `hoststack.namespace-index/<owner>` and bound to the D-080 boot identity. |
| `hoststack.session-rule` | `hoststack.session-rule/<tag>` | `session_rule_add_del`, `session_rules_v2_dump` | yes | The VPP tag is `<owner>:<tag>`, and Retrieve keeps only this owner's rules. `action_index`: allow = `~0-2`, deny = `~0-1`, otherwise a redirect app index. A dump VPP refuses (layer off or plugin missing) yields an empty Retrieve result. |
| `hoststack.tcp-src` | `hoststack.tcp-src/<fib>` | `tcp_configure_src_addresses` | write-only | D-076 applied-once record per VPP boot. There is no delete: Delete drops only the record, so a change is irreversible per fib. |
| `hoststack.http-static` | `hoststack.http-static/global` | `http_static_enable_v5` | write-only | Globals owner only, and only with `VRX_HOSTSTACK_HTTP_STATIC=1` (D-064/D-077 Q3). Applied once per boot. Update is refused while it runs (no disable exists). `www_root` must be under `/var/lib/vrx/www/` (D-049, M5). |

Projection: `internal/desired/host_stack.go` (`services.hostStack` → KVs). A namespace `secretRef` is a DryRun error
(`services.host-stack-secret-channel`) until PENDING-secret-channel lands. `HostStackAssemble` reports only session rules.
RPC: `internal/agent/rpc_host_stack.go` (`HostStackState`, read-only).
