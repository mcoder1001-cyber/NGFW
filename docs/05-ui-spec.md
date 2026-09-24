# Web UI specification (React 19 + MUI v7)

## Screen inventory (~60 screens)

**Dashboard** — throughput & pps graphs, per-worker CPU, session count, hugepage/memory,
interface status strip, active alarms, tunnel health, recent config changes, system info.

**Interfaces** — list (name, type, state, speed, IP, VRF, rx/tx bps+pps, errors) ·
detail/edit · sub-interfaces (802.1q/QinQ) · bonds (LACP) · bridge domains · loopbacks ·
tunnels (GRE/VXLAN/IPIP) · DPDK binding & dataplane tuning wizard.

**Routing** — static routes · VRFs · FIB browser (paged, filterable, 1M rows) ·
BGP: global, neighbors (state, uptime, prefixes, flap count), address-families, route-maps,
prefix-lists, communities, redistribution matrix · OSPFv2/v3: areas, interfaces, neighbors,
LSDB · IS-IS · RIP · BFD sessions · policy-based routing (ACL-based forwarding).

**Firewall/NAT** — address objects & groups, services, schedules · ACL lists & rule editor
(drag-to-reorder, inline edit, bulk import/export CSV, hit counters) · ACL attachments ·
NAT: outbound, 1:1, port-forward, CGNAT pools, MAP-T/E, NPTv6 · session browser with kill.

**VPN** — IPsec tunnels (wizard + advanced), proposals, PSK/cert auth, status with SA/SPI,
rekey history, per-tunnel throughput · WireGuard peers & keys · PKI: CA, certificates,
CSR, import/export, CRL, expiry warnings.

**Services** — DHCP server/relay + lease table + reservations · DNS resolver/forwarder ·
NTP · SNMP · LLDP neighbors · IPFIX/NetFlow exporters · syslog targets.

**System** — hostname/timezone/banner · users, roles, AAA (RADIUS/TACACS+/LDAP/SAML) ·
API keys · TLS certificates · backup/restore/scheduled export · firmware upgrade ·
licence · HA/VRRP · reboot/shutdown · config revisions & diff viewer.

**Tools** — ping, traceroute, packet capture (BPF builder + download), DNS lookup,
log explorer with filters, CLI terminal (xterm.js over WebSocket, admin-only), support bundle.

## Signature UX: the pending-change bar

A persistent bar appears the moment the candidate differs from running:

```
⚠ 7 uncommitted changes   [ Review diff ]  [ Commit… ]  [ Discard ]   locked by: admin
```

`Commit…` opens a dialog with the structured diff, a comment box, and a
**"revert automatically in [2] minutes unless I confirm"** checkbox (default ON for
remote sessions). After commit the UI polls; if it loses the box, it shows a countdown
and "your changes will auto-revert in 1:43 — this is expected if you cut your own access".

Nothing else in the product will earn as much trust from network engineers as this.

## Technical rules

- **Schema-driven forms.** `<SchemaForm schema={ifaceSchema} value={...}/>` renders MUI
  fields from JSON Schema + a UI hints map (widget, order, group, help, dependsOn).
  Hand-written forms only for wizards.
- **Server state:** TanStack Query only. Query keys mirror API paths. Optimistic updates
  on candidate edits; invalidate on commit.
- **Live data:** one `/api/v1/stream` WebSocket, topic subscribe/unsubscribe on mount/unmount,
  auto-reconnect with backoff, buffered at 1 Hz. No component opens its own socket.
- **Big tables:** MUI X DataGrid, `paginationMode="server"`, `sortingMode="server"`,
  `filterMode="server"`. Never fetch 1M routes into the browser.
- **i18n/RTL:** `react-i18next`, namespaces per screen, `en` + `fa` from day one.
  RTL via Emotion + `stylis-plugin-rtl`, `<html dir>` switching, logical CSS properties
  (`margin-inline-start`, not `margin-left`). Persian digits and Jalali date option.
- **Theme:** one `createTheme` with light/dark, 8px spacing, dense tables, a network-status
  colour scale (up/down/degraded/admin-down) defined once as semantic tokens.
- **Charts:** ECharts or Recharts; consistent palette; never more than 8 series; always
  show units (bps vs pps) and let the user switch.
- **Errors:** map RFC 9457 `pointer` → form field error. Global toast only for non-field errors.
- **Performance budget:** first meaningful paint < 1.5 s on the appliance's own web server,
  bundle < 600 kB gzipped initial, route-level code splitting.
