# System identity: hostname, time zone, banners, DNS client

**Screen:** System (the `system` nav entry, `/system`). **API:** the generic configuration routes on `/system`
(`GET/PUT/PATCH /api/v1/config/candidate/system`, `/api/v1/config/system`), then commit.
**CLI:** `vrx configure set|merge system …`, then `commit`.

## What it sets

| field | effect on the router |
|---|---|
| Hostname | `/etc/hostname` and the running host name (RFC 1123, e.g. `vrx-a.lab.example`) |
| Time zone | `/etc/localtime` (IANA name, e.g. `Asia/Tehran`; an unknown zone is refused) |
| Pre-login banner | `/etc/issue` and `/etc/issue.net` (console and SSH before login) |
| Message of the day | `/etc/motd` (after login) |
| DNS client: name servers, search domains | the router's own resolver (systemd-resolved drop-in). NTP is under Services › NTP, and the DNS *server* under Services › DNS |

Banners may hold printable text, line breaks and tabs only. Escape sequences, carriage returns, bell and
bidirectional-override characters are refused, so a banner cannot clear the screen or forge lines (D-049).
The DNS client uses the default VRF only in this release: any other VRF is refused with a pointer to `/system/dns/vrf`.

## Using the screen

1. Open **System**. The form on the left edits the candidate. The table on the right shows each setting as the
   candidate holds it next to what the router runs, and marks rows that are not committed yet.
2. Change fields and press **Save to candidate**. An invalid value is shown at its field (for example
   *unknown IANA time zone* under Time zone).
3. Commit with the pending-change bar.

## Example

```json
PUT /api/v1/config/system
{
  "hostname": "vrx-a.lab.example",
  "timezone": "Asia/Tehran",
  "banner": { "login": "Authorised access only.", "motd": "Welcome to vrx-a." },
  "dns": { "servers": ["192.0.2.53", "2001:db8::53"], "searchDomains": ["lab.example"] }
}
```

CLI equivalent:

```
vrx configure merge system '{"hostname":"vrx-a.lab.example","timezone":"Asia/Tehran","banner":{"login":"Authorised access only.","motd":"Welcome to vrx-a."},"dns":{"servers":["192.0.2.53","2001:db8::53"],"searchDomains":["lab.example"]}}'
vrx commit
```

A refused value comes back as problem+json with a pointer:

```json
{ "status": 400, "title": "Validation failed",
  "errors": [{ "pointer": "/system/timezone", "message": "unknown IANA time zone" }] }
```

## Notes

- A name-server change takes effect when systemd-resolved restarts. The agent writes the configuration and logs the
  restart request, but does not restart the service itself in this release.
- Committing the same settings again changes nothing on the router, and neither does an agent restart.
- Not in this release: the live state endpoint `GET /api/v1/state/system` (uptime, resolver status), and the pre-login
  banner on the web login page (see docs/status/tasks/F-system-identity.md).
