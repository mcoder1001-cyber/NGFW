# DF-8 — review (independent review agent, 2026-09-24)

Branch `task/DF-8` @ a1a34be (base main@c2db9cd, before the DF-2 merge). Scope: 23 descriptors in 9 packages + `descriptors/dfkit`
(+ `dfkittest`, `restarttest`). Checked against REVIEW-PROMPT, the DF-8 factory prompt, the envelope, LOG D-063…D-077, and
vpp-code-track V16–V18. Paths below are relative to `apps/agent/internal/descriptors/` unless they say otherwise.

## What the reviewer ran

- `tools/ci.sh --base main` on a1a34be: **CI GATE PASSED** (quick, 0m56s, logs `/root/ngfw-wt/logs/ci/DF-8-20260924-013047-1386901`).
  The pasted CI output (5c61b1b) is consistent with this run.
- Host integration, slot 5 (`eval "$(tools/lab env 5)"`, `VRX_INTEGRATION=1`), **one package at a time**, without
  `VRX_DF8_HTTP_STATIC`/`VRX_DF8_DUID`: dfkit, dns, dhcp (dhcp6_duid SKIP, opt-in), ipfix, flowprobe, sflow, prom
  (TestHTTPStaticOnHost SKIP, opt-in), pcap, trace, lcp (3/3 subtests ran, **including `replace helpers`**), dfkit/restarttest: all PASS.
  `systemctl show vpp -p NRestarts` = 2 before and after every package.
- Leftovers afterwards (read-only `vppctl show`): no lcp pair, default netns unset, no dhcp proxy/client, flowprobe feature
  empty + params unset, sflow at defaults with 0 interfaces, pcap disabled, bpf filter not set, no DNS servers, no FIB tables
  5800–5999, no `/tmp/w5*`, no `w5-*` netdev.

## Checklist

| # | Item | Result |
|---|---|---|
| 1 | Contract | No hits under packages/schema, packages/proto, apps/agent/gen, api-client generated. OK |
| 2 | Real verification | Every package has a host test that asserts through `Retrieve` (or `ErrRetrieveUnsupported` for write-only types) on the host VPP; CLI evidence pasted. OK |
| 3 | Restart safety | `restarttest` pasted and re-run (fresh connection + descriptors → empty plan; loss → re-create). Every type has Retrieve or a documented write-only reason (D-063). No VPP restart. OK, with gaps in M1, M4 |
| 4 | Binapi provenance | Only `binapi/{dhcp,dhcp6_ia_na_client_cp,dhcp6_pd_client_cp,dns,flowprobe,ipfix_export,sflow,http_static,bpf_trace_filter,lcp,interface,classify,…}` are used. The branch does not touch `binapi/` or `tools/binapi-gen.sh`. OK |
| 5 | Shared-host rules | Loopbacks `loop5xx` are tagged `w5:`/`w5r:`, VRFs 5800–5999, addresses 10.5.x / fd00:5::, host tap `w5-lcp0`, pcap `/tmp/w5-df8.pcap` removed, `t.Cleanup` everywhere, no pkill/daemons. The VPP-global tests break D-071 (M3) |
| 6 | Security | No `exec.Command`/vppctl in the 10 packages. BPF/pcap/lcp/dns inputs are validated by character set. Issues: http_static www_root (M5), pcap file permissions (L1) |
| 7 | Transaction semantics | Mostly sound (D-074 existence checks, ErrRecreate where VPP has no in-place update). Claim-before-add breaks rollback on untagged interfaces (H1) |
| 8/10 | UI / i18n | n/a |
| 9 | Scope creep / dropped types (D-077) | No code is left for tracedump, tracenode, prom-exporter or Trace Path. The `prom` package still holds http_static, which only exists to serve the dropped prom exporter (M5). The docs still describe the dropped types as open questions (L5) |
| 11 | Tests actually run | Yes (see above) |

## Findings (most severe first)

### H1 — The claim is recorded before the add, and an existing object on an untagged interface is adopted, so a foreign object becomes "ours" and is deleted or recreated
`lcp/lcp.go:309-327`, `dhcp/client.go:91-101`, `flowprobe/flowprobe.go:320-323,340-351`, `sflow/sflow.go:386-397`.
`dfkit.ResolveAndClaim` writes the ClaimStore record **before** the VPP add. There are two ways this goes wrong:

1. **The add fails, but the claim stays.** Example: an untagged interface `X` (DPDK NIC, operator/CLI pair, lcp auto-created
   object) already has a pair with `host_if_name=a`. Desired is `host_if_name=b`, so Create returns VALUE_EXIST and the
   comparison does not match. Create returns an error, but the claim `(X, lcp.itf-pair)` stays. On the next resync, Retrieve
   reports the foreign pair as `lcp.itf-pair/X` with value `a`. The diff produces an Update, which returns ErrRecreate, so the
   descriptor **deletes the foreign pair** (and its Linux netdev) and creates its own. The same happens with a dhcp client
   (INVALID_VALUE), flowprobe (ENTRY_ALREADY_EXISTS with another variant) and sflow.
2. **The add hits an identical foreign object and adopts it.** VALUE_EXIST plus an identical dump counts as success
   (lcp 318-326, dhcp 96-99, flowprobe 341-348, sflow 395-396 without any comparison). The owner then Deletes it when the
   object leaves its desired state.

These are the recurring "delete a foreign object" and "untagged interface" patterns. D-071's claim rule assumes the claim
records something *this owner created*.
**Fix:** claim only after a successful add. When the add says "exists", adopt only if this holder's claim was already
present (we created it before an agent restart). Otherwise return an ownership error and do not claim. Release the claim on
every Create error path. Add fake unit tests: a foreign pair/client/flowprobe/sflow on an untagged interface, then Create
fails, then Retrieve must not report it and Delete must not remove it.

### H2 — `ipfix.classify-table` uses a raw VPP classify table index; now that DF-2 is on main this breaks both "index reuse after restart" and "delete by index of a foreign object"
`ipfix/classify.go:35,163-171,174-196`, `ipfix/exporter.go:125`, `dfkit/iface.go` `DefaultClassifyTableKey`.
DF-2 is merged. It keys tables as `classify.table/<name>` and keeps the VPP table index in Meta. DF-8's spec field `Table`
is the **VPP index**, and its dependency key is `classify-table/<index>`. That key is never produced, so the optional
dependency never orders anything, and desired state cannot know the index. The descriptor is write-only (V16), so every
resync re-applies `ipfix_classify_table_add_del(table_id=<n>)`. After a VPP restart, or after DF-2 deletes and recreates a
table, index `<n>` can belong to another table, possibly another owner's. IPFIX reporting then attaches to that table, and
Delete removes a foreign table's IPFIX entry. The prompt's "fixture until DF-2 is merged" has now expired.
**Fix:** reference the DF-2 table by name (`ClassifyTable.Table string`, key `ipfix.classify-table/<name>`, dependency
`classify.table/<name>`, mandatory). Resolve the index through DF-2's store/Meta in Create and again right before Delete
(D-071). Use the scope option only for tests.

### M1 — The sflow hw→sw map survives a VPP restart, and learning races with other enablers
`sflow/sflow.go:310,405-410,477-490`. `learned` lives only in memory and is never tied to the VPP identity. After a VPP
restart (agent still running), DF-1 recreates our loopback and it usually gets the same sw_if_index `Y`. Another slot, or
any foreign enabler, enables sflow on an interface whose hw index is the old `X`. Retrieve then finds `hws={X}` and
`learned[X]=Y`, and `Y` is reportable, so it reports `sflow.interface/<ours>` as **enabled although it is not**. The empty
plan hides the drift permanently. In Create, the before/after dump difference assigns *every* new hw index to our interface.
If another enabler enables something concurrently, a foreign hw index gets mapped to our sw index.
**Fix:** store `iface.VPPIdentity` with `learned` and clear the map when it changes. In Create, learn only when exactly one
new hw index appeared. Add a fake unit test with `RestartVPP()` and a reused index.

### M2 — sflow Retrieve changes the data plane, and after an agent restart it never converges when two or more interfaces are enabled
`sflow/sflow.go:491-501,513-540`. The probe enables and then disables sflow on every owned interface that is not enabled,
whenever an unlearned enabled hw index exists. Each toggle changes the device-input / interface-output / error-drop feature
arcs. On the shared host another slot's sflow is always "unknown", so every resync toggles. On a real box, after an agent
restart with N≥2 enabled interfaces, `len(unknown)==1 && len(found)==1` never holds. Nothing is learned, and every resync,
forever, toggles every non-enabled owned interface.
**Fix:** learn the mapping deterministically once. For each found interface, disable it, dump, and re-enable it (the hw
index that disappears is its hw index). Or persist `learned` (plus the VPP identity) in the state dir. Probe only interfaces
that desired state wants enabled, and never probe on an index owned by someone else.

### M3 — Host tests change getter-less VPP globals on the shared VPP by default and reset them to VPP defaults, against D-071; the global-lock checks and the lcp replace test are check-then-act races
- `dns/integration_test.go:30-39` (enables VPP's resolver and binds UDP 53 for every slot, then disables it regardless of
  its prior state), `trace/integration_test.go:98-100` (removes whatever BPF program exists),
  `pcap/integration_test.go:29-31` (sets the filter function, and Cleanup forces `vnet_is_packet_traced`; this happens
  *before* the busy check at :47, so it also runs when another capture is active), `ipfix/integration_test.go:71-81`
  (classify stream reset to 0). None of these globals has a getter, so "restore previous values, never VPP defaults" (D-071)
  is impossible. By the D-064 / Q3 / Q4 precedent they must be opt-in (`VRX_DF8_GLOBALS=1`, run in a manager window).
- `dfkittest/host.go:123-146` `LockGlobals` is **slot-local**. The read-first checks (lcp default netns
  `lcp/integration_test.go:66-72`, flowprobe params, sflow globals, IPFIX exporter 0) are check-then-act between slots.
  While slot 5 holds `lcp default netns = ns-w5-lcp` (a namespace that does not exist), any other slot's pair with netns ""
  (P12) fails or lands in the wrong namespace.
- `lcp/integration_test.go:86-102` runs `lcp_itf_pair_replace_begin/end` on the shared VPP after a "no pairs exist" check.
  It ran in the reviewer's run. A pair that another slot creates between `Pairs()` and `ReplaceBegin` is deleted by
  `ReplaceEnd`.

**Fix:** make the getter-less global tests opt-in. Run the read-first global tests under an exclusive **lab-wide** lock
(`flock -x` on the lab lock or a shared `/run/lock/vrx-globals.lock`, which is a manager decision). Test the replace helpers
with the fake only, or make them opt-in.

### M4 — The default in-memory BootStore locks the owner out of its own pcap capture / http_static after an agent restart; the tests claim to simulate the restart but do not
`dfkit/boot.go:136-161`, `pcap/pcap.go:218-280`, `prom/prom.go:139-173`. If P05/P08 forget `SetBootStore` (Q8), then after an
agent restart `AppliedThisBoot` is false. `pcap_trace_on` then fails with INVALID_VALUE, which becomes `ErrCaptureBusy`, for
**our own** running capture. `Delete` sees no record, so it never stops the capture. The capture runs until VPP restarts and
the transaction keeps failing. The same happens with http_static (`ErrServerBusy`). `pcap/integration_test.go:55` ("agent
restart, same owner") and `restarttest` construct new descriptors but keep the process-global store, so the case is not
tested.
**Fix:** make the store explicit (a `Register(..., boot dfkit.BootStore)` argument or an option), and fail registration when
none is installed. In tests, simulate the restart with `dfkit.SetBootStore(owner, nil)` and assert the documented behaviour.
Do the same for `iface.SetClaimStore` in `restarttest`.

### M5 — `prom.http-static-server` is left over from the prom exporter that D-077 dropped, and its validation lets VPP serve any directory on any address, irreversibly
`prom/prom.go:71-96,139-184`, `docs/agent/descriptors/prom.md`. The package's only object is http_static, the listener for the
prom page that D-077 dropped. It cannot be disabled or reconfigured. `Validate` accepts any absolute `www_root` (`/`, `/etc`,
`/var/lib/vrx`) and any listen IP (`tcp://0.0.0.0/80`). Once P06/P08 map config onto it, a config value makes VPP (root)
serve the host filesystem on data-plane addresses until the next VPP restart.
**Fix (manager's choice):** (a) drop the package and hand http_static to F-dashboard-prom-alarms together with prom (preferred,
consistent with D-077). Or (b) keep it but restrict `www_root` to an agent-owned directory (for example
`<state dir>/www/…`, symlinks refused) and the URI to 127.0.0.1 or the mgmt address, and rename the package to `httpstatic`.

### M6 — DHCPv4 lease events silently stop after an agent restart
`dhcp/client.go:62-73,145-157,269-293`. VPP binds `dhcp_compl_event` to the **API client_index** of the connection that
configured the client (`dhcp_api.c:302-321`), and it keeps `pid`. After an agent restart (new connection) VPP sends the
events to the stale index, or to whichever client later reuses it, which can be another slot's connection. Retrieve compares
only `want_dhcp_event`, so the plan is empty and the client is never re-registered. `WatchLeases` receives nothing.
**Fix:** decode `pid` from `dhcp_client_details` and treat `WantEvents && pid != os.Getpid()` as drift (Retrieve reports a
different value, which leads to recreate). Or re-configure event clients once per new connection. Add a fake test.

### L1 — The pcap capture file is world-readable, carries no owner prefix, and is never cleaned up
`pcap/pcap.go:111-117`. VPP writes `/tmp/<file>` with `open(..., 0664)` (`vppinfra/pcap.c:70`), so captured payloads
(credentials, DHCP, BGP MD5 input) are readable by every local user. Two owners that choose the same name overwrite each
other's finished capture (O_TRUNC). Nothing removes old captures. The path itself is safe: `/`, `..` and blanks are refused,
and VPP's `unformat_vlib_tmpfile` refuses them too.
**Fix:** require the owner prefix in `File`. Document the permission problem for F-capture-trace, which must move or chmod
the file to 0600 in an agent directory after `pcap_trace_off` and apply a retention policy.

### L2 — Mandatory dependencies on keys only the globals owner registers
`flowprobe/flowprobe.go:302-306` (`flowprobe.params`) and `ipfix/classify.go:169` (`ipfix.classify-stream`). The descriptors
for these keys are registered only by `RegisterGlobals`. A non-owner agent can never satisfy the dependency, and the
"require" mode (D-DF8-11) cannot be reached through `Register`. **Fix:** in `Register`, register the require-mode descriptor
when the agent is not the globals owner, or make the dependency optional and let Create check.

### L3 — The VPP-side host tap of an lcp pair is untagged
`lcp/lcp.go:313-330`. `tap4096` (HostSwIfIndex) carries no owner tag, so DF-1's resolver treats it as an untagged interface
that any owner can resolve by VPP name and claim (dhcp client, flowprobe, even another lcp pair). **Fix:** after the add, tag
HostSwIfIndex `<owner>:<pair>-host` (and re-verify it on Delete). Alternatively, P12 decides.

### L4 — dfkit is a sound base but overlaps with the merged `df2` package
`dfkit` does **not** duplicate DF-4's ClaimStore. It delegates to DF-1's `iface.Claims`, which is the D-069/D-075 path. But:
- `df2` (DF-2, merged) aliases `acl.ClaimStore`, so there are now two claim store families.
- `ParseAddr`/`ToAddress` and atomic file writing exist twice.
- `ErrRetrieveUnsupported` exists twice (`df2/errors.go:19`, `dfkit/errors.go:17`), matched only by text; `errors.Is`
  across them fails.
- `FileBootStore.Put`/`Delete` update memory before a failed flush, and neither syncs to disk.

**Fix (follow-up, D-077 made dfkit the base):** one consolidation task that moves df2 onto dfkit (or both into
`descriptors/kit`). Alias the sentinel to `scheduler.ErrRetrieveUnsupported` when P05 lands.

### L5 — The docs still describe the dropped types as open questions
`docs/agent/descriptors/trace.md:9-11` and `prom.md:9-11` list tracedump/tracenode/Trace Path/prom-exporter as "not
available — see questions". Replace this with "dropped (D-077, V18) — F-capture-trace / F-dashboard-prom-alarms". The
"Open questions" section of `DF-8.md` lists Q1–Q7 and omits Q8. No code is left for these types.

### L6 — dhcp.proxy flip-flops on inconsistent source addresses
`dhcp/proxy.go:176-197,256-277`. VPP sets `dhcp_src_address` only when it creates the proxy of an rx FIB
(`dhcp_proxy.c:184-197`). A second `dhcp.proxy` on the same rx VRF and family with another `src` is accepted. Retrieve then
reports the first src for both, the diff leads to recreate, and the result never converges. **Fix:** reject differing `src`
per (rx VRF, family) in desired-state validation, or key the source per rx VRF.

### L7 — The VPP identity is the main-thread PID
`dfkit/boot.go:163-182` via `iface.VPPIdentity`. In a PID namespace (VPP in a container, where it can be PID 1 every time) the
identity repeats across restarts. Boot records then look current, and the pcap capture / http_static is never re-added. This
is fine on vrx-a. Record it for F-startup-gen / packaging (for example, add the stats-segment epoch or `vpe_pid` together
with a start timestamp).

## Things checked and found correct
- lcp Retrieve and Delete act only on pairs whose **VPP-side** interface is this owner's (tagged, or claimed per H1). Nothing
  touches a Linux netdev or netns directly: the host tap goes away with `lcp_itf_pair_add_del_v3(del)`, which the host run
  verified. The netns and host names are restricted to `[A-Za-z0-9._-]` (no `/`, no `..`, 15/31 bytes).
- The per-VRF dhcp relay (Q7, D-077) is implemented as documented. The scope option filters Retrieve and refuses Create out of
  scope. The DUID is globals-only and its host run is opt-in.
- dns: write-only, idempotent (VPP ignores a duplicate server), globals-only, and the enable/name-server ordering is justified.
- pcap: "clear only what you started" holds while the BootStore is intact. VPP keeps a finished capture enabled, so another
  owner cannot start one in between and be stopped by our Delete.
- The D-074 existence checks before deletes, the index re-resolution before deletes by index, and Dedupe on every Retrieve
  are all present.

**APPROVE WITH CHANGES** — required before merge: H1, H2, M1, M3 (and M5, which is a scope decision for the manager); M2, M4 and M6 before P05/P08 wire these descriptors; L-items can go to follow-ups.
