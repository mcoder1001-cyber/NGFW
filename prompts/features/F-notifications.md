# Task: F-notifications — Email, Telegram and webhook notifications   (prepend 00-CONTEXT.md)

## Goal
Send alarms and important events to admins outside the UI: email (SMTP), Telegram bot, and generic webhook.

## Inputs to read first
- `F-dashboard-prom-alarms` (alarm/event model), audit log, `apps/api/src/secrets/**`, management VRF handling.

## Contract changes
`management.notifications { channels: [{ name, type: email|telegram|webhook, email?{ smtpHost, port, tls: starttls|tls|none,
username, passwordRef, from, to[] }, telegram?{ botTokenRef, chatIds[] }, webhook?{ url, method, headers, secretRef (HMAC) } }],
rules: [{ events: [alarm severity ≥ X | commit | link state | vpn state | global-blocking fetch failure], channels[], throttle }] }`.

## Scope — build exactly this
1. **Dispatcher** (API side): subscribes to alarms/events, matches rules, sends; retry with backoff, per-rule throttle and
   de-duplication, delivery log with last error. Sends go out through the management VRF.
2. **Channels**: SMTP (STARTTLS/TLS, auth), Telegram Bot API, webhook with JSON body and HMAC signature header.
3. **"Send test"** action per channel.
4. **UI**: Notifications tab in the Management screen (channels, rules, delivery log).
5. **Tests**: unit tests with fake SMTP/HTTP servers (success, auth failure, timeout, retry, throttle); secrets never logged.
6. **Docs**: `docs/user/system/notifications.md`.

## Acceptance (paste the evidence)
- [ ] Test message received on each channel type against local fake servers (pasted)
- [ ] Alarm storm of 100 events → throttled to the configured rate (pasted)
- [ ] Tokens/passwords never in GET responses, logs or audit (grep)
- [ ] `tools/ci.sh --base main` green

## Out of scope
SMS, scheduled reports (BL-OPS-08).
