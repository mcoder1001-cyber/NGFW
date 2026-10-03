# Notifications

Configure SMTP email or signed HTTPS webhook channels in Management → Notifications, then rules selecting alarms, commits, link state, WireGuard and strongSwan/IPsec VPN state or global-blocking fetch failures. Telegram is not supported. Save the candidate and commit it before sending a channel test: tests use running configuration and require administrator access.

SMTP requires validated TLS or STARTTLS; credentials use encrypted secret references. Configure the username and password reference together; omitting both explicitly selects an anonymous relay. Webhooks require HTTPS and a secret reference. The POST body is JSON, with `x-vrx-signature: sha256=<hex>` containing HMAC-SHA256 over the exact request bytes. Redirects and non-public destinations are rejected; SMTP may use an on-premise relay. Delivery history uses sanitized error codes and never exposes provider responses or passwords.

Rules throttle repeated events; delivery retries at most three times. The in-memory queue is bounded at 256 entries and history at 500. Configuration reload pauses the worker and aborts active sends; queued work is revalidated against the latest running configuration. Commit notices are coalesced while configuration loads. Queues and delivery history do not persist across API restart.

Only the default management VRF is supported. Schema validation and the transport reject nondefault VRFs before secret access, DNS or socket creation; sends follow the API process network namespace routing. Nondefault VRF routing remains a code gap, so the notification feature is still incomplete. Webhooks use a fixed POST method and signature header; custom methods and headers are not supported.

The strongSwan adapter accepts validated up/down, rekey, daemon and poll attributes from the existing agent event contract. A resync marker does not itself generate a notification; the subsequent poll supplies recovered tunnel state. Unknown or malformed attributes are ignored.

Local tests exercise SMTP TLS and STARTTLS, authentication failure, cancellation and certificate rejection, plus HTTPS HMAC delivery and status handling, using temporary certificates and loopback fake sinks. These tests do not establish appliance routing or real provider acceptance. Live SMTP/webhook delivery, browser workflows, agent integration, routing and database restart acceptance remain deferred.
