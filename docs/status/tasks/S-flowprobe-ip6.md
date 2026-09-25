# S-flowprobe-ip6

What: D-146 follow-up — flowprobe interface default `ip6: false`, ip4+ip6 on one interface is a semantic error at the
`ip6` field; agent warning path kept. Details: `S-flowprobe-ip6-contract.md`, D-150.

Verification (real output):
- `npx turbo run lint typecheck test` (schema, proto, api, web): `Tasks: 23 successful, 23 total`
  (schema 1254, proto 74, api 137, web 113 tests passed).
- `go test ./...` in apps/agent: all packages ok; drift guard: `927 scalar leaves and 208 messages compared,
  4 accepted difference(s), 0 finding(s)`.
- `tools/ci.sh check`: `check PASSED`.

Out of scope: agent code (warning path intentionally unchanged); no VPP in the cloud container, no integration run.
Open questions: `l2` combined with ip4/ip6 is still applied by agent precedence + warning, not rejected in the schema.
