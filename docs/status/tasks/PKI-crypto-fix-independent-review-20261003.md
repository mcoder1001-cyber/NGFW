# Independent PKI crypto repair review — 2026-10-03

Reviewed immutable `542b30337efe395534b92d2930384393a8ebd21a` in PKI-crypto-fix against the three findings in `27d56be086c4fe87e8e3dda3fcf567a455061cbd`. Product files read only. Verdict: **APPROVE source fixes, pending root integration validation**. No remaining blocker from those three findings was established.

## DER containing-element limits

Recursive parseAt receives the parent content end. Its short header, long length header and content bounds all use that limit. A child cannot read either a sibling header or its payload beyond its containing TLV. The original eight-byte overlap fixture and two truncated-header variants are covered, alongside a valid nested sibling regression. Top-level exact-consumption and nesting depth checks remain intact.

## Shared synchronous KDF budget

parsePkcs12 creates one 100,000-round budget per request and passes it through the MAC, encrypted ContentInfo parts, and shrouded key bags. PBKDF2 charges iterations multiplied by derived digest-block count before pbkdf2Sync. PKCS12 KDF charges the same weight before hash allocation/iteration. SHA1 legacy 3DES key derivation charges two blocks and IV derivation charges another block on that same budget; two-key 3DES charges one key block plus one IV block. Neither legacy decrypt path resets the budget. The MAC uses the same request budget and runs before decryption, preserving existing authentication order. Per-operation and aggregate rejection include a useful lower-iteration re-export instruction.

The budget is enforced before each KDF operation, rather than by pre-scanning the whole archive: accepted earlier operations may run before a later operation is refused, but their combined charged cost never exceeds the cap. New tests reject over-limit MAC iterations, multiblock PBKDF2 and aggregate second-decryption overflow; ordinary 2048-round parts still reach content validation. Existing OpenSSL import/KDF vector and RC2 refusal tests remain present. Limits: the new legacy regression covers the direct KDF limit, not an end-to-end cumulative 3DES archive; legacy aggregate accounting was independently traced in source. A round budget bounds digest count, not a measured latency SLA or a guarantee of zero event-loop pause.

## OCSP freshness and consistency

Without nextUpdate, both thisUpdate and producedAt must be at most 24 hours old plus five minutes skew. producedAt cannot exceed the current time plus skew; thisUpdate cannot exceed either current time or producedAt plus skew. With nextUpdate, reversed ranges are rejected and existing expiry/skew semantics remain. Matching CertID and CA/delegated signature verification still precede returning status. Deterministic signed fixtures cover historical replay, a recent response, maximum-age overflow, future dates, reversed intervals and existing nextUpdate acceptance.

Limits: no suites, high-iteration probes, generators, network operations or live services were executed by this reviewer. Parent-owned finite validation must run the new boundary tests plus existing PKCS12/revocation/PKI regressions and scoped lint/typecheck on the combined tree. Delegated responder certificate validity and broader inherited CRL semantics remain previously documented follow-up scope; this repair does not claim to close them.
