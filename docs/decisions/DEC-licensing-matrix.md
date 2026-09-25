# DEC: licensing-matrix (resolved)

- raised: 2026-09-25 by F-licensing
- decision: **2026-09-25, product owner: every gated feature requires a licence, apply now.** `COMMUNITY` = no gated feature, every limit 0; running configuration stays grandfathered (D-144) and the 30-day grace stays. The embedded key remains a placeholder: release engineering sets `VRX_LICENSE_PUBLIC_KEYS` or replaces the key (D-151).
- parked tasks: none

## Context
F-licensing enforces entitlements at commit validation (offline Ed25519 `.vrxlic`). Two things only the product owner
can decide: (1) the entitlement matrix — which features/limits are "community" (no licence) vs paid; (2) the real
product signing key (generated offline, custody with support), which replaces the placeholder public key in
`apps/api/src/features/licensing/licensing.config.ts` (its private half was discarded, so no licence verifies today).
Until answered, `COMMUNITY` = every gated feature, no limits, so nothing is rejected; `SAMPLE_COMMUNITY`
(wireguard ≤ 2 interfaces + ospf) is test data and the docs table only.

## Options
| # | Option | Cost now (agent-h) | Reversal cost (agent-h) | Risk |
|---|---|---|---|---|
| 1 | Keep permissive community; decide matrix later (one table edit in `entitlements.ts`) | 0 | 0.5 | none — no enforcement until decided |
| 2 | Adopt the SAMPLE matrix now (ipsec/bgp/isis/ha paid) | 0.2 | 0.5 | unlicensed boxes reject new IPsec/BGP/IS-IS/HA commits |
| 3 | Different matrix / limits supplied by the product owner | 0.5 | 0.5 | — |

Signing key: generate with `tools/license/vrx-license keygen` on an offline workstation, put the public PEM into
`PRODUCT_PUBLIC_KEYS` — release-engineering step, ~0.2 h.

## Recommendation
Option 1 now; product owner supplies the matrix (option 3 or 2) and the key before the first customer licence.

## What continues meanwhile
Everything: F-licensing merges with permissive enforcement; no task is parked.

## Decision
2026-09-25, product owner: every gated feature (ipsec, wireguard, bgp, ospf, isis, ha) requires a licence; apply now
(a stricter variant of option 3). Implemented by S-license-all: `COMMUNITY` is empty with zero limits, grandfathering and
grace unchanged; new env var `VRX_LICENSE_PUBLIC_KEYS` replaces the embedded placeholder key list. LOG: D-151.
