# Notifications PR66 — independent R6 review

Reviewed local product `97f6863056c630069f48750104ca8d4d8392fe7a`, tree `03d0466129e90c91c00d7ac1cec7f7ed088a5793`; manager identifies remote product as `426b1603`. Isolated branch `review/notifications-r6`. Owned file: this report only. Read AGENTS, shared context, contributing/decision policy, review/R6 prompts, UI specification, notification prompt, user docs and WIP.

## Findings

1. **MAJOR — nested field translations do not resolve.** `apps/web/src/locales/{en,fa}/management.json:55` and `apps/web/src/domains/system/management/NotificationsTab.tsx:81`. SchemaForm resolves complete property paths without array indices (`packages/ui-kit/src/schema-form/text.ts`); nested channel/rule fields request `field.notifications.channels.name.title`, `channels.email.smtpHost.title`, `rules.throttleSec.title`, etc. The new locales instead define `field.notifications.name`, `smtpHost`, and `throttleSec` at the root. A populated Persian editor therefore falls back to English for almost all actual fields; enum labels are absent too. Reorganize both locale trees to match schema paths and supply translated channel type, TLS, event and severity enum maps. Add a populated Persian channel/rule regression.
2. **MAJOR — server field validation is not attached to the form.** `NotificationsTab.tsx:73-85`. PATCH validation errors (for example an unknown rule channel at `/management/notifications/rules/0/channels/0`) only appear in a separate ProblemAlert. SchemaForm never receives its `problem` prop and cannot mark the offending input. Follow the existing sibling TlsTab pattern: convert ApiError with `toFormProblem`, strip `/management/notifications` from pointers, pass it to SchemaForm, and verify a server field error is associated with its input. Keep non-field errors visible.
3. **MINOR — localized formatting bypassed.** `NotificationsTab.tsx:99,119`. Queue count is plain interpolation and delivery time is raw ISO text. Use the existing `useFormatters()` number/dateTime methods so Persian digits, timezone/calendar preferences match surrounding screens.
4. **MINOR — history lacks column headings and explicit empty state.** `NotificationsTab.tsx:113-125`. Empty successful history renders an empty table; populated history has no column header associations. Add translated column headers and a localized no-deliveries state after successful loading.

## Checks and evidence

- Admin-only form and test-send controls correctly use `perms.role !== 'admin'`; backend action declares `@MinRole('admin')`. The supplied operator regression exercises disabled Save. No extra suite result is claimed here.
- Candidate GET/PATCH and operational state/action paths correspond to real endpoints. The screen explains that testing uses enabled running channels and that queued does not mean delivered. Buttons are enumerated from candidate channel names; uncommitted additions can receive the documented running-channel 404, and a staged deletion hides its running channel's test button. This is a usability limitation, not a false send-success claim.
- Uses shared SchemaForm and MUI controls. No added physical left/right CSS or directional icons. Loading and errors exist. Actual rendered RTL/browser acceptance **NOT RUN**; no screenshot names available.
- User docs accurately exclude Telegram and disclose process-network routing, unimplemented non-default VRF binding/IPsec mapping, volatile queue/history, and deferred live/browser/restart acceptance. These remain incomplete product/acceptance scope; review does not promote them to PASS.
- `git rev-parse HEAD HEAD^{tree}` printed the exact local SHA/tree above.
- Ran a Python standard-library lookup over both management locale JSON files for the eight real schema paths below. All sixteen lookups printed `MISSING`:

```text
channels.name.title
channels.type.title
channels.email.smtpHost.title
channels.webhook.secretRef.title
rules.name.title
rules.events.enum
rules.minSeverity.enum
rules.throttleSec.title
```

Lookup command logic: parse each `apps/web/src/locales/{en,fa}/management.json`, select `['field']['notifications']`, then traverse each dotted path using dict.get; print language/path and MISSING for absent values. Source inspection of SchemaForm text.ts and composites.tsx confirms these are the requested paths.

No broad gate duplication. Hosted quick gate is pending; manager-provided ManagementPage 7/7 result is prior evidence, not a command run by this reviewer. Required correction is scoped to real UI/i18n defects; deferred live-browser evidence is not used as a blocker under owner instructions.

**Verdict: BLOCK** — two MAJOR findings pending correction and independent verification; two MINOR findings.
