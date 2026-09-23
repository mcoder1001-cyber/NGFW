# Task: RF-<n> — Renderers for daemons: <daemon list>   (prepend 00-CONTEXT.md)

## Goal
Write the agent-side **renderer** for each daemon in <daemon list>: desired state → validated
config files → applied through the daemon's own control channel → `Retrieve` from the daemon's
JSON/show output → events. Same pattern for every GPL daemon; P11 (strongSwan) is the reference.

## Inputs to read first
- `apps/agent/internal/renderers/README.md` and the `strongswan/` renderer if merged
- Daemon docs: <links>. Installed on this host (disabled): frr, strongswan (stock; vrx build comes from P11), kea-dhcp4/6 + kea-ctrl-agent, unbound, chrony, snmpd, keepalived, rsyslog
- `packages/proto` messages for the domain (P03) — the input type

## Scope — build exactly this, per daemon
1. Templates in `internal/renderers/<daemon>/templates/*.tmpl` rendered with `text/template` and **strict escaping helpers** — no user string reaches the file unescaped.
2. `Validate()` — the daemon's own dry-run (`vtysh -C -f`, `kea-dhcp4 -t`, `unbound-checkconf`, `chronyd -p`? — use what exists; document when none exists).
3. `Apply()` — write files atomically (temp + rename, correct owner/mode), then apply via the control channel: FRR `frr-reload.py --reload`, Kea control-agent HTTP `config-set`, Unbound `unbound-control reload`, chrony `chronyc reload sources`, keepalived `systemctl reload`, snmpd/rsyslog `systemctl reload/restart`. **Fixed argv, no shell**; list every binary you invoke in `internal/renderers/ALLOWLIST.md`.
4. `Retrieve()` — daemon state as structured data (`vtysh -c "... json"`, Kea `lease4-get-all`/`config-get`, `unbound-control stats_noreset`, `chronyc -c sources`).
5. Events where the daemon exposes them (poll at 1 Hz otherwise).
6. Unit tests with golden files (`testdata/*.golden`) — every template path covered, including hostile strings.
7. Integration test on this host: enable the daemon in a **test-scoped** way (start it, apply, Retrieve, stop) — you have exclusive ownership of <daemon list> for this task; nobody else starts them.

## Acceptance (paste the evidence)
- [ ] `go test ./internal/renderers/<daemon>/...` green, integration included
- [ ] `grep -rn "sh -c\|bash -c" internal/renderers/<daemon>` is empty; `ALLOWLIST.md` updated
- [ ] A rendered config with `"; rm -rf /` in a description field is rejected or escaped (test present)
- [ ] Daemon left **stopped and disabled** after tests

## Out of scope (do not build)
API/UI, schema changes, FRR routing-protocol semantics (that is P12/F-*), strongSwan build (P11).
