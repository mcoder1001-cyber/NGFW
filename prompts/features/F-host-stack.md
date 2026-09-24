# Task: F-host-stack — VPP host stack: session layer, app namespaces, session rules, TCP source addresses (scoped down)   (prepend 00-CONTEXT.md)

## Goal
Expose the **API-configurable** part of VPP's host stack end to end in FAST MODE: session-layer enable, application namespaces, session
rules (the host-stack "firewall"), TCP source-address pools, and — opt-in only — the `http_static` server. WBS D7.10 (T3, "no router
customer asks for this") is 130 PD; this task has 10 h, so everything that is startup.conf-only or has no binary API is **documented,
not built**. Reference: VPP "Host Stack" docs; plugins `session`, `tcp`, `udp`, `http_static`.

## Inputs to read first
- `apps/agent/binapi/session/` — `session_enable_disable_v2`, `app_namespace_add_del_v4`, `session_rule_add_del`, `session_rules_v2_dump`
  (verify each name in binapi); `apps/agent/binapi/tcp/` (`tcp_configure_src_addresses` — no dump); `apps/agent/binapi/http_static/`
  (`http_static_enable_v5` — no disable, no dump); `apps/agent/binapi/tls_openssl/` (engine selection only)
- `apps/agent/internal/descriptors/dfkit/` — base helpers (D-077 Q5), `globals.go` (session enable and http_static are VPP globals:
  globals owner only, D-071; tests hold `flock /run/lock/vrx-globals.lock` exclusively and restore the previous value, D-082)
- LOG D-077 Q3 (http_static host tests stay opt-in and run only in a manager VPP-restart window — you cannot undo an enable), D-063/D-076/
  D-080 (write-only rules), D-064 (crash-prone calls opt-in); DF-8 review M5 (http_static `www_root` must be validated)
- `packages/schema/src/domains/services.ts` — **no host-stack model exists**
- `prompts/features/F-startup-gen.md` — TCP/UDP buffer sizes, `session { evt_qs_memfd_seg … }`, `tcp { cc-algo … }` are startup.conf
  sections owned by that generator, not by you

## Contract changes
Additive on `contract/F-host-stack`: `services.hostStack?{enabled, namespaces{<name>:{secretRef?, interface?, vrf}}, sessionRules[{scope:
global|local, transport: tcp|udp, local, remote, action: allow|deny|redirect, appNamespace?, tag}], tcpSourceAddresses?{first,last,vrf},
httpStatic?{enabled, wwwRootPath, uri, cacheSizeMb}}` (secrets via `<kind>/<name>` refs, D-051) + proto. Buffer/cc tuning fields are
proposed to F-startup-gen in your questions file, not added by you.

## Scope — build exactly this
1. **Schema**: namespace secret is a secretRef (never inline); rule prefixes canonical and family-consistent; `wwwRootPath` under
   `/var/lib/vrx/www/` only, no `..`, no control chars (D-049).
2. **Agent**: new `descriptors/hoststack/` package: `hoststack.session` (global, globals owner only), `hoststack.namespace/<name>`,
   `hoststack.session-rule/<tag>` (Retrieve from `session_rules_v2_dump`), `hoststack.tcp-src` (write-only, D-076 applied-once record),
   `hoststack.http-static` (write-only, **not in default Register** — opt-in `VRX_HOSTSTACK_HTTP_STATIC=1`, D-064). Fake-client tests incl.
   duplicate-add; ONE host check for namespaces + session rules only (prefixed tags): Retrieve == desired, `vppctl show session rules`
   contains it, rollback clears, restart simulation.
3. **API**: config via pointer routes; `GET /api/v1/state/host-stack` (session layer on/off, namespaces, rule count).
4. **UI**: one "Host stack" page under Services: enable switch, namespaces, session-rules grid; en+fa; banner "advanced / T3".
5. **Docs**: `docs/user/services/host-stack.md` — what is configurable, and the explicit have-not list below.

Files you own: `apps/agent/internal/descriptors/hoststack/**`, `docs/agent/descriptors/hoststack.md`, `apps/agent/internal/agent/project_host_stack*.go`,
`apps/api/src/features/host-stack/**`, `apps/web/src/domains/services/host-stack/**`, `apps/web/src/locales/*/host-stack.json`,
`docs/user/services/host-stack.md`, `test/topology/host-stack/**`.
Shared files: one-line appends only (agent registry/projection hook, `app.module.ts`, web router/nav); `packages/api-client` regenerated.

## Acceptance (paste the evidence)
- [ ] `vppctl show session rules` / `show app ns` reflect the commit; Retrieve == desired (pasted)
- [ ] Agent-restart simulation → namespaces + rules back within 30 s (log excerpt)
- [ ] Rollback removes rules and namespaces (Retrieve); session layer itself left as the globals owner found it
- [ ] Inline secret or `..` in `wwwRootPath` → 400 problem+json with `pointer`
- [ ] UI screenshot against the real endpoint in `docs/status/tasks/F-host-stack.md`
- [ ] `tools/ci.sh --base main` green in your worktree

## Out of scope (do not build)
VCL/LD_PRELOAD, TLS engine tuning beyond documenting `tls_openssl_set_engine`, QUIC/quicly, HTTP/3 CONNECT and UDP proxying, SRTP, HSI,
TCP/UDP buffer and congestion-control tuning (startup.conf — F-startup-gen), SDL/Auto-SDL knobs (F-rpf-adl-pbr), the prom exporter over
http_static (F-dashboard-prom-alarms), load balancer (F-lb), any new VPP code (a V-item if something is truly needed).

## Open questions to surface, not to decide silently
Whether the product should ship host-stack configuration at all in the 21-day scope (T3) — list it as partial in STATUS-FINAL.
http_static cannot be disabled via API — keep opt-in or drop?
