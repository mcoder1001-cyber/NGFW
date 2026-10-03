# Management: users, AAA, API TLS, remote syslog

**Screen:** System › Management (`/system/management`), one tab per part of the `management` configuration domain.
The old address `/system/users` opens the **Users** tab. **API:** the generic configuration routes on `/management`
(`PATCH /api/v1/config/management`, then commit), plus `GET /api/v1/state/management/tls` for the certificate in use.

| tab | what it edits | status |
|---|---|---|
| Users | `management.users` — local accounts, roles, SSH keys (admin only) | available |
| AAA | `management.aaa` — RADIUS / TACACS+ login | **not yet available** (F-aaa). The tab says so. Only local users can log in |
| API TLS | `management.tls` — certificate and key for the API, minimum TLS version | available |
| Remote syslog | `management.syslog` — collectors the router forwards its log to | available. The same screen as Services › Logging |

## API TLS

The certificate and its private key are never typed into the configuration. You store them in the secret store
(**System › Secrets**, kinds `cert` and `key`) and the configuration holds only references such as `cert/api` and `key/api`.

1. Store the PEM certificate chain (leaf first) as a `cert` secret and the unencrypted PEM private key as a `key` secret.
2. On the **API TLS** tab, choose both references and the minimum version (1.2 or 1.3), then **Save to candidate**.
3. Commit with the pending-change bar.

At commit time the pair is checked before anything is stored:

- both secrets exist and parse as PEM
- the private key matches the certificate
- the certificate is valid now (not expired, not before its start date)
- the TLS library accepts them with the chosen minimum version

A failure is refused with `400` and a pointer to the field (`/management/tls/privateKeyRef` for a key that does not
match, `/management/tls/certificateRef` for an expired certificate). The message never contains key material.

After a commit, a confirm or a revert, the API re-reads the running configuration and swaps the certificate in place.
New connections get the new certificate and protocol floor. Open connections and the service keep running, with no restart.
If a committed certificate cannot be loaded later (for example the secret was replaced by a bad value), the previous
certificate stays in use and the tab shows the error.

The right-hand panel shows the certificate in use: subject, issuer, alternative names, validity, days left, SHA-256
fingerprint and the revision it was loaded from. The private key is never returned by any route.

### HTTPS listener

The API serves HTTPS with this certificate only when `NGFW_HTTPS_PORT` is set in its environment (same bind address as
`NGFW_HTTP_HOST`). Without it, the certificate is still checked and loaded, and the tab says that HTTPS is not being
served. If no certificate is configured, the HTTPS listener does not start. No self-signed certificate is generated in
this release. If the first valid certificate is committed after startup, the listener starts then without
restarting the API. The listener status reports whether the server is actually listening, rather than whether a
port was requested. Reloads run in sequence so slow earlier secret reads cannot overwrite later committed certificates.
Removing the certificate references still leaves an existing listener using its previous certificate until restart;
that removal behavior remains an open lifecycle issue. While retained, the state still shows the certificate
actually served and its loaded revision, with `configured: false`. A failed listener bind reports disabled and
can be retried on a subsequent configuration reload once the requested port is available. The same HTTPS listener accepts `wss://<host>:<port>/api/v1/stream` through the existing
stream authentication and subscription handlers. Missing or invalid credentials are refused before the WebSocket
upgrade. The web UI on :8080 is a separate front end: fronting it with TLS is outside this screen.

Stopping the API closes its active HTTPS stream connections and refuses new stream upgrades once shutdown begins.
Certificate hot reload keeps existing connections open; an API restart disconnects them, so clients reconnect after
the service becomes available again.

اگر نخستین گواهی معتبر پس از شروع API ثبت شود، شنوندهٔ HTTPS بدون راه‌اندازی مجدد فعال می‌شود.
وضعیت listener نشان می‌دهد که سرور واقعاً در حال گوش‌دادن است؛ تنظیم پورت به‌تنهایی به معنی فعال‌بودن نیست.
اگر پورت در دسترس نباشد، بارگذاری مجدد بعدی می‌تواند شروع شنونده را دوباره امتحان کند.
حذف ارجاع‌های گواهی، گواهیِ شنوندهٔ فعال را تا راه‌اندازی مجدد کنار نمی‌گذارد؛ این رفتار هنوز مسئلهٔ باز چرخهٔ عمر است.
در این حالت، `configured: false` همراه با مشخصات گواهیِ واقعاً ارائه‌شده و نسخهٔ بارگذاری‌شده نمایش داده می‌شود.
مسیر `wss://<host>:<port>/api/v1/stream` از همان احراز هویت و اشتراک‌های موجود استفاده می‌کند؛
درخواست فاقد اعتبار یا دارای اعتبار نامعتبر پیش از ارتقای WebSocket رد می‌شود.

با شروع خاموش‌شدن API، اتصال جدید به جریان HTTPS پذیرفته نمی‌شود و اتصال‌های فعال آن بسته می‌شوند.
تعویض گواهی بدون راه‌اندازی مجدد، اتصال‌های موجود را حفظ می‌کند؛ پس از راه‌اندازی مجدد API باید دوباره متصل شوید.

## Remote syslog

Each collector has an address, port, transport (UDP, TCP or TLS), minimum severity and VRF, plus the forwarding
options described in [Services › Logging](../services/unbound-chrony-syslog.md). The tab is the same screen as
Services › Logging, including live forwarding counters.

## Example

```sh
# store the pair (admin; POST /api/v1/secrets, the value is write-only)
jq -n --rawfile v api.crt '{kind:"cert",name:"api",value:$v}' | \
  curl -s -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d @- http://127.0.0.1:3000/api/v1/secrets
jq -n --rawfile v api.key '{kind:"key",name:"api",value:$v}' | \
  curl -s -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d @- http://127.0.0.1:3000/api/v1/secrets
ngfw configure merge management '{"tls":{"certificateRef":"cert/api","privateKeyRef":"key/api","minVersion":"1.3"}}'
ngfw commit
curl -s -H "Authorization: Bearer $TOKEN" http://127.0.0.1:3000/api/v1/state/management/tls   # certificate in use (never the key)
```
