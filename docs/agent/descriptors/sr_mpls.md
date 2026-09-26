# sr_mpls (SR-MPLS) descriptors (DF-6, WBS D6.7 / D2.8)

Package `apps/agent/internal/descriptors/sr_mpls`. Messages only from `apps/agent/binapi/sr_mpls` (+ `mpls`, `ip` for probes).
Shared rules: [df6.md](df6.md).

| Object type | Descriptor / key | Create / Delete | Retrieve | Update | Dependencies |
|---|---|---|---|---|---|
| SR-MPLS policy | `sr-mpls.policy` · `sr-mpls.policy/<bsid>` | `sr_mpls_policy_add` + `sr_mpls_policy_mod` (ADD) per further list / `sr_mpls_policy_del` | **partial: write-only** | `ErrRecreate` | `mpls-table/0` (DF-7 key) |
| SR-MPLS steering (by BSID) | `sr-mpls.steering` · `sr-mpls.steering/<table>/<prefix>` | `sr_mpls_steering_add_del` | **partial: write-only** | `ErrRecreate` | `sr-mpls.policy/<bsid>`, `vrf/<table>` |
| Endpoint + color | `sr-mpls.endpoint-color` · `…/<bsid>` | `sr_mpls_policy_assign_endpoint_color` once per VPP boot; Delete = documented no-op (cleared by `sr_mpls_policy_del`) | **write-only** | re-assign in place | `sr-mpls.policy/<bsid>` (must be ours) |

Models: `sr_mpls.Policy{bsid, spray, segment_lists[{labels ≤16, weight 1–255}] (ascending)}`,
`sr_mpls.Steering{prefix, table_id, bsid, vpn_label (0 = none)}`, `sr_mpls.EndpointColor{bsid, endpoint, color}`.

Why write-only (partial): binapi `sr_mpls` has **no dump**. The fallback the task names — derive the policy from
`mpls_route_dump` of the BSID label — does not work: VPP encodes recursive MPLS paths without their via-label (the
first segment of every list; `fib_api_path_encode` sets no `via_label`), and the path list reorders the lists. So
Retrieve returns `df6.ErrRetrieveUnsupported`, never cached desired state. Ownership is a claim (D-071). Presence is
probed exactly and keeps Create/Delete idempotent across resyncs (D-076): a policy is the end-of-stack entry of the
BSID label in MPLS table 0 whose every path is a recursive MPLS path (proto MPLS, no interface, type normal) — an
ordinary MPLS route (DF-7) does not match; a steering entry is a `FIB_SOURCE_SR` route (`fib_source_dump` name "SR",
`ip_route_v2_dump` with that source) with a recursive MPLS path in its table (VPN label compared as identity).

Guards: empty segment lists are rejected (VPP reads `segments[0]` of an empty vector); steering needs the policy to
exist (VPP leaves a half-created steering entry otherwise) and the table to exist (unchecked `fib_table_find` on add
and delete). Color-based automated steering (bsid `~0`, next-hop + color) is not modelled. The endpoint-color host
test is skipped (global TE MPLS table + internal labels, no un-assign); unit-tested on the fake.

## F-mpls-srmpls

- `sr-mpls.endpoint-color` declares `CheckPersistent` (TD-11b, `ownership.go`): its per-boot claims are keyed by BSID and
  need a persisted store keyed by id — `df6.WithClaims(Wiring.PairClaims("df6"))`; policy and steering are
  `df6.KeyedDescriptor`s and declare the same check. Test: `f_mpls_srmpls_test.go/TestOwnershipDeclarations`.
- Product wiring (`subsystems/mpls_srmpls.go`): `sr_mpls.Register(r, c, owner, df6.WithClaims(PairClaims("df6")),
  df6.WithGlobalsOwner(...))`. Policy and steering are in `Domains[routing]`; endpoint-color is registered but in no
  domain (SR-TE color steering is not configurable), so it is never planned.
- `sr-mpls.policy` depends on `mpls-table/0`; the projection declares it (the D-071 role decides create vs require).
  Segment lists are sorted ascending by the projection (D-074).
