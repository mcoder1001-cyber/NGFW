# S-flowprobe-ip6 — contract change (D-150, supersedes the D-146 interim)

Commit `contract(schema): flowprobe interface records ip4 only by default`:

- `IpfixFlowprobeInterfaceSchema.ip6` default `true` → `false` (`packages/schema/src/domains/services.ts`); the
  schema default is now one variant (ip4), which VPP can record. Regenerated
  `packages/api-client/src/generated/schema.d.ts` (`@default false`). No proto change (proto3 bool, no default).
- New semantic validator `services.ipfix-sflow-flowprobe-one-variant`: `ip4` and `ip6` both true on one interface is
  an error at `/services/ipfix/flowprobe/interfaces/<i>/ip6`.
- Agent unchanged: the ip4 + `agent.unsupported-field` warning path stays as defence for documents that bypass the
  API validator.
- Documents relying on the old default: the P02c example (`services-snmp-lldp-ipfix-ntp.json`, no explicit flags)
  now parses as ip4 only — valid, no edit needed; `packages/proto/test/fixtures/all-domains.json` set `ip6: false`.
  A stored document with explicit `ip4: true, ip6: true` is now refused at commit (behaviour change, intended).
- Callers adjusted: api fake (`ip6` default false), web `variantOf` (`ip6 === true`) + its test,
  `docs/user/services/ipfix-sflow.md`.

Verification: turbo lint/typecheck/test for schema, proto, api, web — 23/23 tasks (schema 1254, proto 74, api 137,
web 113 tests); `go test ./...` in apps/agent all ok; drift guard 927 leaves, 0 findings; `tools/ci.sh check` (see
commit). Cloud container: no VPP — no integration run.
