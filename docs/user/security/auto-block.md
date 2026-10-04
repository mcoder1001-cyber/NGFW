# Auto-block (brute-force and scan protection)

Auto-block watches the box's own login and control planes and **temporarily blocks a source IP** that fails too many
logins, or scans the box, in a short window. It protects the web UI / API, SSH, VPN (IKE/EAP) logins and the box's
own ports without you touching a firewall rule.

Blocking is **dynamic**: an offending source is added to a system-owned entry in the Global Blocking engine with a
time-to-live and removed automatically when the block expires — no configuration commit happens per block.

## Turn it on

**Config › Security › Auto-block**

| Field | Meaning |
| --- | --- |
| **Enabled** | Master switch. When off, nothing is auto-blocked. |
| **Detectors** | One rule per source (see below). A source with no rule is not watched. |
| **Allow list** | Addresses and prefixes that are **never** blocked — your management networks and your own address. Loopback is always allow-listed. |
| **Maximum blocked entries** | Cap on the live set; the soonest-to-expire entry is dropped when the cap is reached. |

### A detector rule

| Field | Meaning |
| --- | --- |
| **Detector** | `webLogin` (web / API sign-in), `ssh`, `vpnAuth` (IKE/EAP), or `portScan` (probes to the box). |
| **Threshold** | Failures (for `portScan`, distinct ports) from one source within the window before it is blocked. |
| **Window** | The sliding window the threshold is counted over. |
| **Block** | How long a first offence is blocked. |
| **Escalate on repeat** | Double the block time on each repeat offence, up to the cap. |
| **Maximum block** | The escalation cap — a single block never lasts longer than this. |

A sensible start for the web UI: threshold 5, window 60 s, block 900 s, escalate on, cap 24 h.

## The allow list always wins

A source that matches the allow list (or loopback) is **never** blocked, and it cannot be blocked by hand either. This
is how you make sure the tool can never lock **you** out — put your management network in the allow list. Because the
allow list is checked before anything is counted, an allow-listed client never even accrues failures.

## Watching and clearing blocks

**Firewall › Auto-block** lists everything blocked right now: the source, which detector caught it, how many hits and
how many repeat offences, when it was blocked and when it expires.

- **Unblock** removes a source now. This is temporary — a source that keeps offending is blocked again. To stop a
  source from ever being blocked, add it to the allow list in **Config › Security**.
- **Block by hand** lets you block an address yourself (for an hour by default). An allow-listed address is refused.

Every block and unblock is recorded as a system event (subsystem `security`) and published on the `security.events`
stream, so alarms and notifications can pick them up.

## What runs where

The web/API login detector runs in the management service. The API publishes its complete PostgreSQL runtime set
through `AutoBlockSet` over the existing agent socket, without a configuration commit. Snapshots are serialized,
republished after reconnect and retried every five seconds. The agent caches them privately, filters expired or
allowlisted entries using its own clock, and reuses Global Blocking for VPP ACLs and nftables local-in. Runtime updates
never change the running/candidate document. Removing a block or changing the allowlist republishes the entire set.
The agent expires blocks even if the API is unavailable and recreates them during its usual VPP resync.

`auto-block` is reserved as a system-owned Global Blocking list. The current host supports at most 20,000 entries,
keeping a complete IPv6 snapshot below the existing gRPC 4-MiB receive boundary. A `security.autoBlock.maxEntries` above that limit is refused during
agent validation; entries are never silently discarded. The default remains 10,000. Management source ranges in
`acl.hostSettings.antiLockout.sources`, configured allowlist sources, IPv4-mapped aliases, and loopback are protected.

The product agent reads bounded trusted journal pages for SSH failures (root-owned sshd/sshd-session records). Port
scan detection observes distinct destination ports from kernel nftables records, within the configured window.
Its observer hook runs after the block chain and before ordinary local-in policy. Journal logs are limited to 100
packets/second per protocol with a 200-packet burst; under heavier floods, scan observations may be incomplete.
The observer accepts only its own chain; ordinary host policy still applies in later chains.

Native VPP IKEv2 VPN failures are detected from the existing secret-safe SA reader's `AUTH_FAILED` state, once per
owned profile/SPI pair. The source must match the configured remote endpoint paired with the local endpoint; IKE
identities never supply block addresses. This requires the established secret-safe native state capability. If that
capability is unavailable, the agent reports the detector unavailable and does not fall back to raw key-bearing
state dumps. Polling can miss short-lived failed SAs that disappear between polls. EAP is not supported by the
native route-based VPN feature. Charon IKE/EAP parsing is retained for compatibility tests, but charon records never
publish product VPN block observations: strongSwan is restricted to test peers and foreign daemons cannot authorize
a native product block. Only the owned native VPP state watcher publishes `vpnAuth`.

The real packet acceptance driver is documented in `test/topology/autoblock/README.md`. Unit and fake-VPP checks
verify projection, cache replay, rollback and expiry; they do not substitute for real local-in/forwarding acceptance.

## Out of scope

IPS signatures and honeypots are separate features. Auto-block is about rate-based brute-force and scan blocking only.
