# WEB-2 — questions for the manager

## Q1 — `tools/ci.sh` contract guard is flaky: `pipefail` + `git log | grep -q` (owner P09; not edited)

`do_contract_guard` decides with `if git log --format=%s "$mb..$TIP" | grep -qiE '^contract(\(|:|!)'; then`. Under
`set -o pipefail`, `grep -q` exits at the first match. When `git log` is still writing, it gets SIGPIPE and the pipeline
status is 141, so the guard reports "CONTRACT FILES CHANGED WITHOUT A CONTRACT COMMIT" although the branch has contract
commits. This affects every branch based on `task/P08`, because P08's four `contract(...)` commits sit mid-log. On
this branch:

```
$ for i in 1 2 3 4 5 6 7 8; do tools/ci.sh check --base main >chk.log 2>&1; echo "$? $(grep -c 'WITHOUT A CONTRACT COMMIT' chk.log)"; done
1 1
0 0
1 1
1 1
1 1
0 0
1 1
0 0
$ bash -c 'set -o pipefail; n=0; for i in $(seq 1 200); do git log --format=%s 63178d2..HEAD | grep -qiE "^contract(\(|:|!)" || n=$((n+1)); done; echo "pipeline false in $n/200 runs"'
pipeline false in 110/200 runs
```

Suggested fix (one line, P09's file): read the whole log first, e.g.
`if git log --format=%s "$mb..$TIP" | grep -iE '^contract(\(|:|!)' >/dev/null; then`
(no `-q`, so grep drains the pipe), or `subjects=$(git log …); grep -qiE … <<<"$subjects"`. In WEB-2.md, only guard failures
were retried, never another step.

## Q2 — Registering the Secrets page and the previews (no `// web: WEB-2` anchor on main)

The envelope allows router/nav lines only under `// web: WEB-2` anchors. Neither main nor `task/W-seed` has one, so the
Secrets page and the dev previews are **not registered**. `apps/web/src/pages/dev/previews.ts` exports
`DEV_PREVIEWS` (dev builds only). Once the anchors exist, these lines finish the wiring:

```ts
// router.tsx, feature screens:     { path: 'system/secrets', lazy: async () => ({ Component: (await import('./pages/SecretsPage')).SecretsPage }) },
// router.tsx, DEV_ROUTE_OBJECTS:   ...DEV_PREVIEWS.map((p) => p.route),
// nav.ts, system group:            { id: 'secrets', path: '/system/secrets', labelKey: 'config:kit.secrets.title', fallbackLabel: 'Secrets', available: true },
// nav.ts, DEV_NAV_ITEMS:           ...DEV_PREVIEWS.map(({ id, path, labelKey, fallbackLabel }) => ({ id, path, labelKey, fallbackLabel, available: true })),
```

The Secrets page runs on the real P06 API (`/api/v1/secrets`), so routing it adds no stub. When it gets routed, it
should also get an E2E step in `test/e2e/flow.e2e.mjs` (create → rotate → delete, with a `VRX_TEST_PSK_<id>` value) and a
`docs/user/` page. Both files are outside WEB-2's ownership.

## Q3 — Preview strings reach the production bundle

`config:kit.preview.*` holds 9 short strings. The `config` namespace is always loaded, but review P07a M1 keeps dev-only
strings in the `dev` namespace, which is loaded only with `DEV_ROUTES`. WEB-2 owns only `config.json` (`kit.*`). The code
and chunk of the previews are still excluded from a production build: `DEV_PREVIEWS` is `[]` and nothing imports it. Move
the 9 strings to `dev.json` when previews.ts is wired, or allow WEB-2 to edit `dev.json`.

## Q4 — Moving P08 interfaces and System › Users onto the kit (follow-up; not done here)

Rewriting `domains/interfaces/*` and `pages/UsersPage.tsx` onto the kit is outside WEB-2's files. The generic parts are
extracted into `config/collection` and `config/widgets`. `config/collection/equivalence.test.ts` checks that the kit
gives the same results as each screen's own helper on the same inputs. Deliberate differences are listed in WEB-2.md
§3. A follow-up task (owner of those files) can switch them over and delete the per-screen copies, and with them the
matching blocks of the equivalence test.

## Q5 — `GET /api/v1/secrets` does not publish `version` (additive contract candidate)

At runtime the list already carries `version`. The real-API check (WEB-2.md §4.4) got
`{"ref":"psk/w1-web2","kind":"psk","version":2,…}`, but the controller's OpenAPI `SecretOut` omits the field, so the
generated client type lacks it. The page uses generated types only, so it cannot show a secret's current version; it
shows the version only right after a store, taken from the POST answer. The fix is additive: add
`version: z.number().int()` to `SecretOut` in `apps/api/src/secrets/secrets.controller.ts` and regenerate the client in
a `contract(api-client)` commit. WEB-2 does not own that file.
