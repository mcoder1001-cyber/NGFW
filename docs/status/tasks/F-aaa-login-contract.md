# F-aaa-login — contract changes (additive)

## contract(schema): aaa oidc
- `packages/schema/src/domains/management.ts`: `AuthMethod` widened in place with `oidc`; `AaaSchema` gets one key line
  `oidc: aaaOidcField` under the `// wave-BC: F-aaa` anchor and two refines (oidc in `order` needs the block,
  path `oidc`; scopes must include `openid`, path `oidc/scopes`).
- `packages/schema/src/domains/ext/aaa.ts`: `AaaOidcSchema` — `issuer`, `clientId`, `clientSecretRef` (`token/<name>`
  secret reference, never the secret), `redirectUri`, `scopes` (default openid/profile/email), `usernameClaim`
  (default `preferred_username`), `roleClaim` (default `groups`). The block is optional (absent by default), so
  `AaaSchema.parse({})` is unchanged. URLs are https; plain http only for a loopback host (development IdP).
- Test: `packages/schema/src/semantic/aaa-oidc.test.ts`.

## contract(proto): aaa oidc mirror
- `ManagementAaa.oidc = 5` (the number reserved for it in docs/status/wave-BC-numbers.md "F-aaa"); new message
  `AaaOidc` (1 issuer, 2 client_id, 3 client_secret_ref, 4 redirect_uri, 5 scopes, 6 username_claim, 7 role_claim)
  in the `// ----- F-aaa -----` section. The secret reference is a plain string (like `bind_password_ref`). Field 6
  (saml) stays reserved. Regenerated with `packages/proto/gen.sh` (apps/agent/gen, packages/proto/gen/ts). The agent
  ignores these leaves (D-040).
- Drift guard: `go test ./internal/contracttest/` ok.
