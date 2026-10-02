# Notifications R6 — correction verification

This supersedes the initial BLOCK in `notifications-resume-review-R6.md`, which remains preserved unchanged. Independently inspected corrections `0db184e3` and lint-only follow-up `66b2bc95`; cherry-picked onto the isolated R6 review branch as `09bd2168` and `a883dca1`. No reviewer-authored product edits.

## Resolved findings

1. Nested en/fa titles now match `channels.*`, `channels.email.*`, `channels.webhook.*` and `rules.*` SchemaForm paths. Channel type, TLS mode, event and severity enum maps have real Persian labels. Populated Persian regression exercises SMTP host, channel/rule labels and enum presentation.
2. `notificationProblem()` converts ApiError field messages and strips `/management/notifications` only at an exact path boundary, for both top-level pointer and errors[]. SchemaForm receives the result. The regression verifies the actual channel-name input has `aria-invalid=true` and its accessible description contains the server validation message; errors are not merely displayed elsewhere. Non-field ProblemAlert remains.
3. Queue count uses `fmt.number`, delivery time uses `fmt.dateTime`; default raw-number/ISO rendering is removed.
4. Delivery history has translated column headers and explicit translated empty state after successful loading.

Administrator controls, real endpoint use, candidate/running explanation, logical styling and honest unsupported VRF/IPsec scope remain as reviewed initially. No new R6 findings.

## Actual verification

Ran independently in `/workspace/scratch/de92de7d9874/NGFW-notifications` at `66b2bc952bff1a7d8822acfedc8b2aeaf78454d2`, reusing its installed dependencies without product edits:

```sh
PATH=/workspace/scratch/96b8b6fbc8a7/toolchain/bin:$PATH pnpm --filter @ngfw/web exec vitest run src/domains/system/management/ManagementPage.test.tsx
```

```text
✓ src/domains/system/management/ManagementPage.test.tsx (9 tests) 5589ms
Test Files  1 passed (1)
Tests       9 passed (9)
Start at    18:27:03
Duration    8.77s
```

An existing router `No HydrateFallback element provided` warning was emitted; no failed tests. This includes the operator Save guard, populated Persian fields and associated server-field error regressions. Reviewed the sole difference between first correction and tested checkout: static COLUMNS keys moved out of JSX with no behavior change. All reviewed UI paths match the corrected isolated worktree.

Re-ran Python JSON lookup of the same eight formerly missing nested paths, followed by recursive full management-locale leaf-key comparison:

```text
en: all 8 previously missing paths now present
fa: all 8 previously missing paths now present
en/fa management leaf-key parity: PASS
```

No broad gate rerun. Hosted quick gate remains a separate mandatory merge requirement. Live browser/RTL screenshot and real SMTP/webhook/routing/restart acceptance **NOT RUN**, with no screenshot names claimed. UI tests use the fake API and do not establish real delivery or full feature acceptance.

**Verdict: APPROVE** for R6 on the corrected product (`0db184e3` + `66b2bc95`), zero outstanding R6 findings.
