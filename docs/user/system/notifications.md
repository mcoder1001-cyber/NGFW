# Notifications

Configure SMTP email or signed HTTPS webhook channels in Management → Notifications, then rules selecting alarms, commits, link state, WireGuard or strongSwan/IPsec VPN state, or global-blocking fetch failures. Telegram is not supported. Save the candidate and commit it before sending a channel test: tests use running configuration and require administrator access.

SMTP requires validated TLS or STARTTLS; credentials use encrypted secret references. Webhooks require HTTPS and a secret reference. The POST body is JSON, with `x-vrx-signature: sha256=<hex>` containing HMAC-SHA256 over the exact request bytes. Redirects and non-public destinations are rejected; SMTP may use an on-premise relay. Delivery history uses sanitized error codes and never exposes provider responses or passwords.

Rules throttle repeated events; delivery retries at most three times. The in-memory queue is bounded at 256 entries and history at 500. Queued work is revalidated when configuration changes. Queues do not persist across API restart.

Outbound sends follow the API process network namespace routing. Explicit non-default management VRF socket binding is not implemented. The dispatcher handles strongSwan/IPsec tunnel transitions, rekeys, daemon state and recovered polling events as VPN notices. Live SMTP/webhook delivery, browser workflows, routing and database restart acceptance remain deferred; local unit results do not prove them.

## Set up and verify a channel

1. As an administrator, store the SMTP password or webhook signing key in System → Secrets. Enter its reference in the channel configuration.
2. Add a channel in Management → Notifications, enable it, then add a rule naming that channel and choosing event classes, minimum severity and throttle interval.
3. Save the candidate and commit the pending changes. Saving alone does not activate the new channel. A channel removed only from the candidate remains available for testing until the deletion is committed.
4. Use **Send test** for an enabled running channel. “Test queued” means the dispatcher accepted the request; check Delivery history for **Sent**, **Failed** or **Discarded** to learn what happened.

If active channels are still loading, wait for that request to finish. If it fails, the screen shows the API error and does not offer tests using candidate channels as a fallback. A disabled channel is not offered for testing. Non-admin users can inspect notification settings and delivery history; they cannot change these settings or send tests.

## Troubleshoot delivery

| Result or error | What to check |
| --- | --- |
| Test queued, no final result yet | Refresh delivery history after the worker finishes; acceptance into the queue does not confirm delivery. |
| `secret-unavailable` | Check that the secret reference exists and has the correct kind. Enter secret values only in the secret store. |
| `destination-rejected` | Check the destination and DNS records. Webhooks require a public HTTPS destination; redirects, loopback and private webhook destinations are rejected. SMTP can use a private relay, subject to its address policy. |
| `smtp-failed` | Check relay reachability, TLS trust, account credentials and allowed sender/recipient addresses. Provider responses are intentionally excluded from public history. |
| `http-4xx` or `http-5xx` | Check the webhook endpoint's access and processing rules, including verification of the configured signing key. The history records the returned status code. |
| `delivery-timeout` or `transport-failed` | Check outbound routing, name resolution and endpoint availability. Failed attempts can be retried up to three total attempts. |
| `configuration-unavailable` or `configuration-read-timeout` | The dispatcher cannot load running configuration; restore API/datastore availability before testing again. |
| Discarded | A queued event may no longer match an enabled channel/rule after configuration changes, or its attempt was cancelled. Inspect the running configuration before retrying. |

[راهنمای فارسی](notifications.fa.md)
