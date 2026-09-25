# Task: F-host-stack — VPP host stack: session layer, app namespaces, session rules, TCP source addresses (scoped down)   (prepend 00-CONTEXT.md)

## Goal
Expose the **API-configurable** part of VPP's host stack end to end in FAST MODE: session-layer enable, application namespaces, session
rules (the host-stack "firewall"), TCP source-address pools, and — opt-in only — the `http_static` server. WBS D7.10 (T3, "no router
customer asks for this") is 130 PD; this task has 10 h, so everything that is startup.conf-only or has no binary API is **documented,
not built**. Reference: VPP "Host Stack" docs; plugins `session`, `tcp`, `udp`, `http_static`.

## Inputs to read first
- `apps/agent/binapi/session/` — `session_enable_disable_v2`, `app_namespace_add_del_v4`, `session_rule_add_del`, `session_rules_v2_dump`
  (all present, checked 2026-09-24); `apps/agent/binapi/tcp/` (`tcp_configure_src_addresses` — no dump, no delete flag); `apps/agent/binapi/http_static/`
  (`http_static_enable_v5` — no disable, no dump); `apps/agent/binapi/tls_openssl/` (engine selection only)
- binapi facts (checked 2026-09-24): there is **no app-namespace dump** (only `app_namespace_add_del{,_v2,_v3,_v4}`) → `hoststack.namespace`
  is write-only too (D-063; idempotent re-add or a D-076 applied-once record). `session_enable_disable_v2` carries only `rt_engine_type`
  (`DISABLE | RULE_TABLE | NONE | SDL`) and has no getter: session rules need the **rule-table** engine, F-rpf-adl-pbr's Auto-SDL
  (`services.autoSdl`, ServicesConfig 8) needs **sdl** — one VPP-wide engine, so one document cannot have both (semantic rule, see Scope)
- the host VPP's session layer is whatever startup.conf left it (handover-gated, D-012): **never enable, disable or re-engine the session
  layer on the shared VPP** (same rule as F-rpf-adl-pbr). Check read-only (`vppctl show session verbose`); if it is off, the host check below
  is skipped with that reason and everything stays fake-tested
- `apps/agent/internal/descriptors/dfkit/` — base helpers (D-077 Q5), `globals.go` (session enable and http_static are VPP globals:
  globals owner only, D-071; tests hold `flock /run/lock/vrx-globals.lock` exclusively and restore the previous value, D-082)
- LOG D-077 Q3 (http_static host tests stay opt-in and run only in a manager VPP-restart window — you cannot undo an enable), D-063/D-076/
  D-080 (write-only rules), D-064 (crash-prone calls opt-in); DF-8 review M5 (http_static `www_root` must be validated)
- `packages/schema/src/domains/services.ts` — **no host-stack model exists**
- F-startup-gen is merged: TCP/UDP buffer sizes, `session { evt_qs_memfd_seg … }`, `tcp { cc-algo … }` are startup.conf sections of the
  generator `apps/agent/internal/renderers/vppstartup/` (read-only for you; it renders no `session`/`tcp` stanza today), not yours
- `docs/decisions/PENDING-secret-channel.md` — no API→agent secret channel exists yet: a namespace `secretRef` cannot be resolved by the agent

## Contract changes
Additive, as separate `contract(schema): …` / `contract(proto): …` commits on **your task branch** (no own branches; field numbers from
your envelope / `docs/status/wave-BC-numbers.md`): `services.hostStack?{enabled, namespaces{<name>:{secretRef?, interface?, vrf}}, sessionRules[{scope:
global|local, transport: tcp|udp, local, remote, action: allow|deny|redirect, appNamespace?, tag}], tcpSourceAddresses?{first,last,vrf},
httpStatic?{enabled, wwwRootPath, uri, cacheSizeMb}}` (secrets via `<kind>/<name>` refs, D-051) + proto + drift guard. Buffer/cc tuning
fields are proposed for the start-up generator in your questions file, not added by you.

## Scope — build exactly this
1. **Schema**: namespace secret is a secretRef (never inline); rule prefixes canonical and family-consistent; `wwwRootPath` under
   `/var/lib/vrx/www/` only, no `..`, no control chars (D-049); session rules and `services.autoSdl.enabled` in one document → 400 with a
   `pointer` (one rt engine, see Inputs).
2. **Agent**: new `descriptors/hoststack/` package (on `descriptors/dfkit`): `hoststack.session` (global, **write-only**, registered only for
   the globals owner — D-071), `hoststack.namespace/<name>` (write-only, no dump), `hoststack.session-rule/<tag>` (Retrieve from
   `session_rules_v2_dump`), `hoststack.tcp-src` (write-only, D-076 applied-once record), `hoststack.http-static` (write-only, **not in default
   Register** — opt-in `VRX_HOSTSTACK_HTTP_STATIC=1`, D-064). A namespace with a `secretRef` is refused with a clear DryRun error until the
   secret channel lands (PENDING-secret-channel). Fake-client tests incl. duplicate-add; ONE host check for namespaces + session rules only
   (prefixed tags), **only if the host session layer is already on with the rule-table engine**: Retrieve == desired, `vppctl show session
   rules` contains it, rollback clears, restart simulation.
3. **API**: config via pointer routes; `GET /api/v1/state/host-stack` (session layer on/off, namespaces, rule count).
4. **UI**: one "Host stack" page under Services: enable switch, namespaces, session-rules grid; en+fa; banner "advanced / T3".
5. **Docs**: `docs/user/services/host-stack.md` — what is configurable, and the explicit have-not list below.

Files you own and shared hotspots: your TASK ENVELOPE is authoritative (the board's old `agent/project_host_stack*.go` became
`internal/desired/host_stack*.go` + `internal/subsystems/host_stack*.go` + `internal/agent/rpc_host_stack*.go`, wave-A hotspots A2).
Shared files: registration lines under your anchor only (agent registry/projection hook, `app.module.ts`, web router/nav, services tab
registry); generated files regenerated, never hand-edited.

## Acceptance (paste the evidence)
- [ ] `vppctl show session rules` / `show app ns` reflect the commit; Retrieve == desired (pasted) — or, if the host session layer is off,
      the skip reason pasted and the same checks on the fake client
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
Session rt engine: rule-table (session rules) vs sdl (Auto-SDL, F-rpf-adl-pbr) — which one does the product's globals owner select by default?
