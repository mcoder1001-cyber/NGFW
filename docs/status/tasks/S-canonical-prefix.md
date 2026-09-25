# S-canonical-prefix — schema parity with TD-16b (D-149) strict prefixes

## What
TD-16b makes the agent reject host-bit prefixes for IPv6 RA prefixes, LISP EIDs, SR / SR-MPLS steering
prefixes and 6rd prefixes. At base be53867 packages/schema only models **LISP** of those five; RA
(F-neighbors-ra), SRv6 (F-srv6), SR-MPLS (F-mpls-srmpls) and 6rd have no schema yet (only wave anchors).

- LISP EIDs: already rejected by semantic rule `tunnels.lisp-eid-canonical` (requires canonical text,
  pre-existing domain policy — also rejects host bits). No code change needed; added an explicit
  per-field test (accept 10.0.0.0/24 and 2001:db8::/64, reject 10.0.0.1/24 and 2001:db8::1/64 on
  localEids and remoteMappings) in packages/schema/src/semantic/lisp.test.ts.

## Verification
`npx turbo run lint typecheck test --filter=@ngfw/schema`:
```
@ngfw/schema:test:  ✓ src/semantic/lisp.test.ts (16 tests)
@ngfw/schema:test:       Tests  1252 passed (1252)
 Tasks:    4 successful, 4 total
```

## Out of scope / open questions
- RA prefixes, SR steering, SR-MPLS steering, 6rd: add a network-address check (`canonicalPrefix`
  host-bit compare, not text compare) when those feature schemas land (F-neighbors-ra, F-srv6,
  F-mpls-srmpls, 6rd owner).
