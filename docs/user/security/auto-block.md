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

The web / API login detector runs in the management service. The SSH, VPN-auth and port-scan detectors, and the
enforcement of blocks on the data plane (VPP ACLs and the host `nftables` local-in chain), run on the box itself and
arrive with the host build; until then the web/API detector and the block list above are fully active for the
management plane.

## Out of scope

IPS signatures and honeypots are separate features. Auto-block is about rate-based brute-force and scan blocking only.
