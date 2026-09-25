# QoS — policers, rate limits and marking (flat QoS)

**Screen:** *Services → QoS* (`/services?tab=qos`). **Configuration:** `services.qos` through the generic
configuration routes (`/api/v1/config/services`). **Live state:** `GET /api/v1/state/services/qos/policers`.
**Action:** `POST /api/v1/actions/qos/policers/{name}/reset`. **CLI:** `vrx set services qos …`, `vrx merge …`
(`docs/user/cli/reference.md`).

VRX configures VPP's flat QoS: **policers** (token-bucket metering with conform / exceed / violate actions),
**rate limits** on egress (the schema's `shapers`), **recording** the DSCP / 802.1p / MPLS EXP bits of received
packets, **storing** a fixed QoS value, and **marking** (rewriting) those bits on egress through **translation maps**.

> **Not in this release (V3):** hierarchical QoS, per-interface queues and schedulers. VPP 26.06 has no queueing
> shaper outside the DPDK HQoS scheduler, which VPP removed. See [the rate-limit caveat](#rate-limits-are-drop-based)
> below.

## Objects

| object | what it is | VPP |
|---|---|---|
| `policers.<name>` | a token-bucket meter: algorithm `1r2c` (one rate, conform/exceed), `1r3c-rfc2697`, `2r3c-rfc2698`, `2r3c-rfc4115`, `2r3c-mef5cf1` (two rates: `cir` + `eir`, `cb` + `eb`); rates in kbit/s (bursts in bytes) or packets/s (bursts in packets); per colour an action `transmit`, `drop` or `mark-and-transmit` (with a DSCP) | `policer_add`, named `<owner>:<name>` (the product: `vrx:<name>`) |
| `shapers.<name>` — shown as **Rate limits (egress)** | `rateKbps` and an optional `burstBytes` | an egress policer `shaper:<name>`: `1r2c`, cir = rate, burst = `burstBytes` or ≈ 10 ms of traffic (at least 3000 bytes), exceed → **drop** |
| `maps.<name>` | a translation table per recorded source: `rows.ip` (DSCP 0–63), `rows.vlan` (802.1p 0–7), `rows.mpls` (EXP 0–7), `rows.ext` (0–255); each entry maps a recorded value (`from`) to an output value (`to`); unlisted values map to 0; optional fixed `id` | `qos_egress_map_update` (VPP egress map id: `id`, else the lowest free id the agent owns, in name order) |
| `interfaces.<if>` | the QoS of one interface (the VPP interface name): `policer.input` (ingress), `policer.output` or `shaper` (egress — one of the two), `record` (ingress: copy the bits of this header into the packet's QoS record), `store` (ingress: give every packet this value; **ip only**), `mark` (egress: rewrite the `output` header from the recorded bits through `map`) | `policer_input` / `policer_output`, `qos_record_enable_disable`, `qos_store_enable_disable`, `qos_mark_enable_disable` |

Validation (400 problem+json with a pointer to the field): unknown policers, shapers, maps or interfaces
(`services.qos-references`); duplicate map ids and marked values that do not fit the output header
(`services.qos-consistency`); `store.source` other than `ip` (`services.qos-flat-store-source`: VPP 26.06 implements
`qos store` for ip only); `mark` without `map` and `shaper` together with `policer.output` are refused by the schema
itself. The agent also refuses a map id outside the id range it owns and a policer name too long for VPP's 63-byte
policer name with the owner prefix (about 56 characters for a policer, 49 for a rate limit).

## Rate limits are drop-based

A "shaper" in VRX is **not** a queueing shaper: it is an egress policer that **drops** every packet above the rate
(VPP has no queue to delay it in). TCP adapts to it, but bursts are cut instead of smoothed, and the burst matters: the
default ≈ 10 ms of traffic (never less than two 1500-byte frames) keeps full-size packets passing at low rates; raise
`burstBytes` for bursty traffic. The screen labels them *Rate limits (egress)* for this reason. A real shaper would
replace the implementation without changing the document (the object stays `shapers`).

## Typical setups

**Ingress policing of a customer port** — 20 Mbit/s committed, 40 Mbit/s peak, excess re-marked AF11 (DSCP 10),
violating traffic dropped:

```json
{"services": {"qos": {
  "policers": {"cust-a": {"type": "2r3c-rfc2698", "cir": 20000, "eir": 40000, "cb": 25000, "eb": 50000,
                          "conformAction": {"action": "transmit"},
                          "exceedAction": {"action": "mark-and-transmit", "dscp": 10},
                          "violateAction": {"action": "drop"}}},
  "interfaces": {"GigabitEthernet0/8/0": {"policer": {"input": "cust-a"}}}
}}}
```

**DSCP remark via a map** — record the DSCP on the inside port, rewrite EF (46) to AF41 (34) and CS1 (8) to 0 on the
outside port:

```json
{"services": {"qos": {
  "maps": {"remark": {"rows": {"ip": [{"from": 46, "to": 34}, {"from": 8, "to": 0}]}}},
  "interfaces": {
    "GigabitEthernet0/8/0": {"record": "ip"},
    "GigabitEthernet0/9/0": {"mark": {"map": "remark", "output": "ip"}}
  }
}}}
```

Marking acts only on packets whose QoS bits were recorded (or stored) on ingress. Values the map does not list map to
0, so list every value that must survive (e.g. `{"from": 0, "to": 0}` is implied, `{"from": 34, "to": 34}` is not).

**DSCP → 802.1p** — record ip on ingress, mark `vlan` on a VLAN sub-interface with a map whose `ip` row gives the PCP
(`{"from": 46, "to": 5}`); marked values must fit the output header (PCP 0–7).

**Egress rate limit** — `"shapers": {"uplink": {"rateKbps": 50000}}` and `"interfaces": {"<if>": {"shaper": "uplink"}}`.

## The screen

- **Policers:** the configured policers with algorithm, rates and bursts, and — from VPP — the conform / exceed /
  violate packet counters and a status (*applied*, *not in VPP*, *not configured*). **Reset** refills the policer's token
  buckets (`policer_reset`); VPP keeps the counters (they restart when the policer is created or changed). The editor
  has a rate/burst helper (burst = rate × time window).
- **Rate limits (egress):** the shapers, with the drop-based caveat and their counters.
- **Marking maps:** a grid per source (64 DSCP, 8 PCP, 8 EXP, 256 ext cells); a cell shows the output value, an empty cell
  maps to 0.
- **Interface attachments:** per interface the ingress / egress policer or rate limit, record, store and mark.

Edits go to the candidate; the pending-change bar at the top shows the diff and commits. The live counters refresh every
30 s and on **Refresh** (the agent walks VPP's policer pool one caller at a time, D-132).

## How it behaves on the data plane

- **Policer attachments are applied once per VPP start.** VPP stacks a second policer instance on every apply and has no
  dump of interface policers, so the agent records each attachment it applied (per VPP boot identity) and never applies
  it twice — not after an agent restart, not on a resync. After a VPP restart it applies them once more. It removes an
  attachment only if it applied it since VPP started (removing one VPP never saw writes out of bounds in VPP 26.06).
- **Retrieve / drift:** policers, rate limits, maps, records, stores and marks are read back from VPP (names and
  descriptions from the agent's own record, D-073b). Policer attachments cannot be read back (write-only, D-063): the
  agent marks them `agent.write-only` so `/state/drift` does not report them.
- **Rollback** removes the policers, maps, marks, records and stores, marks before the maps they use.
- **Maps keep their id** when you set `id`; without one the agent numbers them in name order, so adding a map whose name
  sorts first renumbers the others (the marks follow automatically). Set `id` to avoid that.

## The same with the CLI and REST

```sh
vrx set services qos policers cust-a cir 20000
vrx set services qos policers cust-a cb 25000
vrx set services qos interfaces GigabitEthernet0/8/0 policer input cust-a
vrx merge /services/qos/maps '{"remark": {"rows": {"ip": [{"from": 46, "to": 34}]}}}'
vrx show configuration diff
vrx commit confirm 120
vrx confirm

# live policer state and counters (REST; readonly role is enough)
curl -s -H "authorization: Bearer $T" http://127.0.0.1:3000/api/v1/state/services/qos/policers
# refill the buckets of one policer, or of a rate limit (operator; audited)
curl -s -X POST -H "authorization: Bearer $T" http://127.0.0.1:3000/api/v1/actions/qos/policers/cust-a/reset
curl -s -X POST -H "authorization: Bearer $T" http://127.0.0.1:3000/api/v1/actions/qos/policers/shaper%3Auplink/reset
```

This release has no dedicated `vrx show qos` command; `vrx --json show configuration services qos` shows the
configuration. On the box, VPP's own view: `vppctl show policer`, `vppctl show qos egress map`, `vppctl show qos mark`,
`vppctl show qos record`, `vppctl show qos store`.
