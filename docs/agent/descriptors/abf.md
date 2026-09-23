# abf descriptors (DF-2, WBS D2.4 ACL-based forwarding)

Package `apps/agent/internal/descriptors/abf`. Messages from `apps/agent/binapi/abf` (+ `binapi/acl` to resolve ACL names, `fib_types` paths).

| Object | Descriptor / key | Create / Update / Delete | Retrieve | Dependencies | Notes |
|---|---|---|---|---|---|
| policy | `abf.policy` / `abf.policy/<policy_id>` | `abf_policy_add_del` (policy_id, acl_index, paths); Update: path changes in place (the API is additive: add new paths first, then remove stale ones); ACL change → `ErrRecreate` | `abf_policy_dump` (acl_index → ACL name via `acl_dump` tags, paths decoded to `df2.FibPath` with interface names) | `acl.acl/<name>` (DF-4 key, mandatory), path interfaces (Optional) | No tag: owned by policy id in the agent's `df2.IDRange` **and** an ACL of this owner. Meta `{ACLIndex}`. Pass desired through `NormalizePolicy` (path defaults, sort). |
| interface attach | `abf.attach` / `abf.attach/<policy_id>/<ifname>/<ipv4\|ipv6>` | `abf_itf_attach_add_del` (policy, sw_if_index, priority, is_ipv6); Update → `ErrRecreate` | `abf_itf_attach_dump`, owned policy id + owner-tagged interface | `abf.policy/<id>`, `interface/<ifname>` | Meta `{SwIfIndex}`. |

ACL key contract: DF-4 `docs/agent/descriptors/acl.md` — `acl.acl/<name>`, VPP tag `"<owner>:<name>"`. Until DF-4 merges the tests create the ACL through `binapi/acl` with that tag (`df2test.ACL`).
`abf_plugin_get_version` = health check only. CLI: `show abf policy`, `show abf attach <if>`.
