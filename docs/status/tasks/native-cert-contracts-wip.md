# Native certificate contracts checkpoint

Branch: codex/native-cert-contracts-20261004. Owned: schema/proto/generated Go and TS contracts, API sealed-delivery consumer checks, web IPsec model/locales. No handwritten Go, board or decision changes.

Envelope: owner approved explicit peer leaf pin and one shared local identity; no CA-chain trust assertion. Base origin/main 1af871f0ba380071e4a6ea18b56636489a067433. No host mutation. Focused tests and unchanged mandatory guard required; existing owner full/hosted CI waiver retained.

Completed sources: additive peerCertificate / peer_certificate field 5; enabled native cert validation requires explicit pin and rejects remoteCa; semantic peer refs, imported leaf/private-key requirements and shared local identity.
Actual checks pending generation/tests. Next command: pnpm --filter @ngfw/schema exec vitest run src/domains/vpn.test.ts src/semantic/vpn.test.ts.
Publication pending; no remote SHA claimed.

Follow-up: public-only peer import exposed an existing required PKI key field. Made only PKI certificate privateKeyRef optional (WireGuard remains required); explicit PEM publicOnly mode imports exactly one non-CA leaf, rejects key and CA options before writes, returns keyRef null and stages removal of a prior key reference. Remote-access local cert semantic validation still requires key. IPsec form offers cert authentication and requires peerCertificate; hides unsupported remoteCa. English/Persian UI explains peer leaf public-key pin, no CA-chain or peer-presented-certificate verification and one shared local identity. PKI import checkbox exposes public-only mode.

Actual: contract checkpoint published remote 0e3e137f3dde5d17e214aaa44a44968b4677ab36 / local d7656d31a; schema119 tests passed before follow-up, latest122 passed; API sealed delivery and PKI actions23 tests passed; scheduler schema/proto/YANG/API/client generation9/9 passed. Focused web/typecheck/guard pending. No full CI or host acceptance claimed. Next: tools/ci.sh check --base origin/main.

Final focused validation: schema domain/semantic124/124 PASS; API sealed-delivery+PKI-controller23/23 PASS; IPsec+PKI web11/11 PASS after building workspace prerequisites (initial web import failures were missing local dist dependencies, corrected by scheduler build12/12 PASS). API/web typechecks PASS; schema lint and focused API/web ESLint PASS; unchanged tools/ci.sh check --base origin/main PASS10s; git diff --check PASS. Generation9/9 PASS via scheduler; generated Go/proto/APIclient/YANG written by generators. Full/hosted CI and live lab certificate negotiation were not run.

Product frozen local5e351f43a published exact remote bcd5b28e7159336ec47c9708cd460fb69232d500 on codex/native-cert-contracts-public-only-20261004; first remote0e3e137 retained as parent. Remaining: manager integrates Go consumer and independent review; packet negotiation/restart acceptance stays explicit lab follow-up. Next manager command: git cherry-pick d7656d31a 5e351f43a plus final test/status checkpoint.
