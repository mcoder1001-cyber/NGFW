# Global blocking (IP block lists)

**Firewall › Global blocking** drops traffic from and to lists of IP addresses and prefixes on the interfaces you
choose, **before any access list**. A list comes from a text file you upload, or from a server URL the box downloads —
on a schedule if you give it a refresh interval. IPs only: domain lists, Geo-IP and DNS sinkholes are not part of this
page.

## The list file
Plain text, one entry per line:

```
# comments start with #, blank lines are skipped
192.0.2.7            an address (becomes 192.0.2.7/32)
198.51.100.0/24      a prefix
10.1.2.3/8           host bits are masked: 10.0.0.0/8
2001:DB8::1          IPv6 (written back as 2001:db8::1/128)
```

The box normalises every entry, removes duplicates, and folds prefixes covered by a wider one into it. At most
**200 000 entries** over all lists.

## Settings of a list
| Field | Meaning |
|---|---|
| Enabled | A disabled list stays in the configuration but is not enforced. |
| Source | *Uploaded file*, or *Server URL* (`url`, `refreshSec`, `verifyTls`, `caRef`, `authRef`). |
| All interfaces / Interfaces | Where the list is enforced. *All interfaces* also covers interfaces added later. Naming an interface that does not exist is refused at commit. |
| Direction | `inbound` drops packets **arriving from** a listed address; `outbound` drops packets **leaving to** one; `both` (default) does both. |
| Protect the box | Also drops traffic from listed addresses **to the appliance itself** (SSH, web UI, API, routing sessions…), on every host interface. |
| Log | Drops to the box are logged (`vrx:gb:<list>` in the kernel log); data-plane drops are counted, not logged. |

### Server URL
- `https` verifies the server certificate. For a private CA, store its certificate as `cert/<name>` in
  **System › Secrets** and set *Private CA*. `verifyTls: false` turns the check off (not recommended).
- Credentials are secret references: `token/<name>` is sent as `Authorization: Bearer …`; `password/<name>` holds
  `user:pass` for basic authentication. Never type a password into the list itself.
- The download is capped at 20 MB and 30 s; redirects are not followed.
- Without `refreshSec` the list is downloaded only when you press **Fetch now**.

## Importing
1. **Import file** (or **Fetch now** for a URL list) → **Check**. The preview shows the number of lines and entries,
   what is added and removed, and every invalid line with its line number (invalid lines are skipped).
2. **Stage** writes the entries into the candidate; the file replaces the list's entries.
3. Commit from the pending-change bar. Nothing is enforced before the commit.

**Export** saves the candidate's entries in the same format.

## Scheduled refresh
With `refreshSec` set, the box re-downloads the list when it is due (checked every minute; `If-None-Match` /
`If-Modified-Since` make an unchanged file cheap). A changed file is committed automatically as a **system change**
(revision kind `system`, no author, visible in **System › Revisions**, and rollback-able like any revision).

- **Someone is editing the candidate** (uncommitted changes) or a commit waits for confirmation → the refresh waits
  (*waiting* on the page) and is tried again at the next check; it never commits someone else's edits.
- **Any failure keeps the last good list**: network or TLS error, an HTTP error, an empty file, more than 10 % invalid
  lines, or more than 200 000 entries. The page shows *failed* with the reason, and the system event
  `BLOCKLIST_FETCH_FAILED` is raised (warning). A successful change raises `BLOCKLIST_REFRESHED`.

## What the box does
- **Data plane (VPP):** each list becomes deny ACLs named `_gb.<list>.i<nn>` (inbound) and `_gb.<list>.o<nn>`
  (outbound), put **first** on every selected interface. The entries are spread over up to 64 bucket ACLs per
  direction, so changing a few entries of a large list replaces only the buckets they fall into. On an interface
  that has no access list of its own in a direction, a permit-all ACL `_gb.pass` follows the block lists — VPP
  denies what no ACL of a bound direction matches, so without it the block list would drop everything else.
- **The box itself (nftables, *Protect the box*):** the sets `b4_<list>` / `b6_<list>` and the chain `in__gb`
  (input hook, priority −300: before connection tracking, so blocked sources create no connection-tracking entries;
  established connections from a listed address are dropped too). With **Host ACL › Settings › anti-lockout sources**
  set, management traffic from those sources is accepted first, so a list cannot lock you out; with no sources
  configured it does not (the list would otherwise never block SSH/HTTPS).
- **Drops** on the page: the packets matched by the list's VPP ACLs (all interfaces together; the ACL plugin counts
  per rule, not per interface; shown when the ACL hit counters are on) and the packets dropped on their way to
  the box.
- **Show drift:** the lists and their ACLs are compared like the access lists; a list changed in VPP behind the
  agent's back shows up as a difference under `/acl/globalBlocking`.

## Limits
- One interface takes at most 255 ACLs over both directions (access lists and block-list buckets together); a
  commit that would exceed it is refused (`acl.global-blocking-limit`).
- The host part replaces the whole nftables table on every change (atomic, a few seconds for 200 000 entries).
- The block lists live in the configuration document: a 200 000-entry list makes every revision a few MB.
