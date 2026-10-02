# Notifications

Configure SMTP email or signed HTTPS webhook channels in Management → Notifications, then rules selecting alarms, commits, link state, WireGuard VPN state or global-blocking fetch failures. Telegram is not supported. Save the candidate and commit it before sending a channel test: tests use running configuration and require administrator access.

SMTP requires validated TLS or STARTTLS; credentials use encrypted secret references. Webhooks require HTTPS and a secret reference. The POST body is JSON, with `x-vrx-signature: sha256=<hex>` containing HMAC-SHA256 over the exact request bytes. Redirects and non-public destinations are rejected; SMTP may use an on-premise relay. Delivery history uses sanitized error codes and never exposes provider responses or passwords.

Rules throttle repeated events; delivery retries at most three times. The in-memory queue is bounded at 256 entries and history at 500. Queued work is revalidated when configuration changes. Queues do not persist across API restart.

Outbound sends follow the API process network namespace routing. Explicit non-default management VRF socket binding is not implemented. The strongSwan/IPsec event producer exists, but its notification adapter is not implemented. Live SMTP/webhook delivery, browser workflows, routing and database restart acceptance remain deferred; local unit results do not prove them.
