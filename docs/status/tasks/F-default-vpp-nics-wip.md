# F-default-vpp-nics recovery — 2026-10-04

Branch: `codex/recover-nics-20261004`. Base: `origin/main@980d54be5`.
Owned files: recovered NIC agent, API, UI, schema, additive protobuf contract and generated stubs; associated NIC documentation/tests.
Source: historical merge `79fff64a`, adapted from vrx to ngfw. Current main's identity/IPsec/tunnel protobuf messages and interface tests preserved.

Completed code: read-only HostNics, explicit opt-in default-off seed, management NIC protection, immutable seeded physical markers, release/reclaim UI, state projection and semantic validators. Proto stubs regenerated using `packages/proto/gen.sh`.

Actual checks: schema build + 10 NIC semantic tests PASS; proto generation/build PASS; API typecheck PASS (after building yang dependency); NIC seed + drift 11 tests PASS. Go agent, vppstartup, desired and contracttest packages PASS. Seed golden refreshed solely for four tunnel maps added since historical merge. Full API-client turbo regeneration PASS (9 tasks); client build and web typecheck PASS. Focused seeded physical NIC release UI test PASS (1 targeted, 11 skipped by name filter), including case-insensitive removal of an uppercase existing PCI device key. API/schema scoped ESLint PASS. Independent reviewer identified PCI casing issue; fixed before integration. Full/hosted CI skipped at owner's instruction. Hardware NIC binding/firstboot acceptance not executed on shared host; historical evidence files are historical, not current verification.

Publication: local branch only until manager publishes; remote SHA unavailable. Exact next command: focused check outputs then manager integration and API-client regeneration.
