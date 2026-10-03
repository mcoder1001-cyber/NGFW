# Licensing

NGFW uses a simple, **offline** licence: a signed file that says who the licence is for, how long it is valid and which
features it covers. There is no online activation and no call-home. Licensing never touches the data plane: it is
checked only when you commit a configuration.

## What happens when

| Situation | Status | Effect |
|---|---|---|
| No licence installed | `community` | No gated feature: new configuration of any feature in the table below is refused. |
| Licence valid | `valid` | The licence's features and limits apply. |
| Licence expired, less than 30 days ago | `grace` | Licence still applies; every commit returns a warning; a banner is shown; a `license.grace` system event is recorded. |
| Licence expired more than 30 days ago | `expired` | Community set for **new** configuration; a `license.expired` system event is recorded. |
| Wrong host (binding), not valid yet, or the stored file does not verify | `invalid` | Community set for new configuration. |

**Running configuration is never removed.** When a licence expires, everything already committed stays applied and keeps
forwarding. Only a commit that *adds* an unlicensed feature (a node that is not in the running configuration) is rejected
with `403` `application/problem+json`, `type …/license-required`, `tier: "license"` and `errors[].pointer` naming the
first offending node, for example `/ha/vrrp/lan-v4`.

## Entitlement table

**Every feature below requires a licence** (product owner decision, `docs/decisions/DEC-licensing-matrix.md`). An
unlicensed device (no licence, `invalid`, or `expired` past grace) refuses any commit that *adds* configuration of these
features; the community set is empty with every limit at 0 (`COMMUNITY` in `entitlements.ts`). Configuration already
running is grandfathered and keeps working.

| Feature id | Used when the configuration has | Without licence |
|---|---|---|
| `ipsec` | `/vpn/ipsec/tunnels/*` | refused |
| `wireguard` | `/vpn/wireguard/interfaces/*` | refused |
| `bgp` | `/routing/bgp` | refused |
| `ospf` | `/routing/ospf` | refused |
| `isis` | `/routing/isis` | refused |
| `ha` | `/ha/vrrp/*`, `/ha/cluster` | refused |

| Limit id | Counts | Missing in the licence |
|---|---|---|
| `ipsecTunnels` | `/vpn/ipsec/tunnels/*` | unlimited |
| `wireguardInterfaces` | `/vpn/wireguard/interfaces/*` | unlimited |

Everything else (interfaces, static routing, NAT, ACLs, services, …) is never gated. The table lives in one place:
`apps/api/src/features/licensing/entitlements.ts`.

## Web UI, REST and CLI

- **System → Licence**: status, customer, expiry, days left, the entitlement table, and *Upload licence file* (admin).
- `GET /api/v1/state/license` (any role): status, entitlements in force, days left and the customer name. The signature
  and the binding values are never returned.
- `PUT /api/v1/system/license` (admin, audited): body = the `.ngfwlic` file content. A malformed, tampered or wrongly
  signed file, a file bound to another host, and a file expired past grace are rejected with `400`.
- CLI (`ngfw`): the generated operations for these two routes (see `docs/user/cli/reference.md`). The CLI enforces
  nothing itself; the API does.

## File format (`.ngfwlic`)

```json
{
  "format": "ngfwlic/1",
  "license": {
    "version": 1,
    "licenseId": "LIC-2026-0001",
    "customer": "Example Ltd",
    "issuedAt": "2026-09-25T00:00:00.000Z",
    "notBefore": "2026-09-25T00:00:00.000Z",
    "expiresAt": "2027-09-25T00:00:00.000Z",
    "binding": { "serial": "VMware-42 1a 2b ..." },
    "entitlements": { "features": ["ipsec", "ha"], "limits": { "ipsecTunnels": 50 } }
  },
  "signature": "<base64 Ed25519 signature>"
}
```

`signature` is a detached Ed25519 signature over the **canonical JSON** of `license` (object keys sorted recursively,
no whitespace). `binding` is optional: `serial` (DMI product serial, `/sys/class/dmi/id/product_serial`) and/or
`machineIdHash` (SHA-256 hex of `/etc/machine-id`). Note that VM templates clone `/etc/machine-id`; prefer the serial.

The API trusts the product public key(s) compiled into `apps/api/src/features/licensing/licensing.config.ts`, or, when
`NGFW_LICENSE_PUBLIC_KEYS` is set (comma-separated PEM public keys; `\n` escapes allowed), exactly those keys instead.
**Release engineering must set `NGFW_LICENSE_PUBLIC_KEYS` or replace the embedded key**: the embedded key is a
placeholder whose private half was discarded, so without that step no licence verifies and every device stays
unlicensed. For development only, `NGFW_LICENSE_PUBKEY_FILE` adds one more trusted key. The installed file is stored at
`NGFW_LICENSE_FILE` (default `/var/lib/ngfw/license.ngfwlic`). `NGFW_LICENSE_SERIAL` overrides the detected serial.

## How support issues a licence

`tools/license/ngfw-license` needs only Node 22 (no packages):

```sh
# once, on the offline signing workstation — never inside a git checkout (the tool refuses)
tools/license/ngfw-license keygen --out-dir /secure/ngfw-license-keys
# → ngfw-license-signing.pem (0600, keep offline) and ngfw-license-public.pem (goes into licensing.config.ts)

# the customer sends the serial:  cat /sys/class/dmi/id/product_serial
tools/license/ngfw-license issue --key /secure/ngfw-license-keys/ngfw-license-signing.pem \
  --customer "Example Ltd" --id LIC-2026-0001 --days 365 --serial "VMware-42 1a 2b ..." \
  --features ipsec,ha --limit ipsecTunnels=50 --out example.ngfwlic

tools/license/ngfw-license verify --pub /secure/ngfw-license-keys/ngfw-license-public.pem example.ngfwlic
tools/license/ngfw-license inspect example.ngfwlic
```

The customer uploads `example.ngfwlic` on **System → Licence**.

### Bash shortcut

For the easiest full-feature licence, run:

```bash
bash tools/license/generate-license.sh
# Optional customer name and output path:
bash tools/license/generate-license.sh all "Example Ltd" example.ngfwlic
```

This creates keys on first use, reuses them on later runs, enables all six
features with unlimited counts, and verifies the output. It defaults to 365 days
(override with `NGFW_LICENSE_DAYS`). Existing output files are never overwritten.
The generated `license.ngfwlic.api.env` contains a public-key setting compatible
with shell and systemd: put its line in `/etc/ngfw/api.env`, replacing any existing
`NGFW_LICENSE_PUBLIC_KEYS` entry, restart `ngfw-api`, and upload `license.ngfwlic`
in **System → Licence**. This changes the API's trusted product keys; keep an
existing signing pair if licences already in use must continue verifying.
No API settings or services are changed by the generator itself.

`tools/license/generate-license.sh` provides editable defaults: keys under
`$HOME/.config/ngfw/license-keys`, a 365-day duration, and all six licensed features
with no limits. It uses the same Node issuer and signature format as above.

```bash
tools/license/generate-license.sh init
tools/license/generate-license.sh api-env
# Apply the printed export to the API process environment and restart the API.
tools/license/generate-license.sh issue --customer "Example Ltd" --out example.ngfwlic
tools/license/generate-license.sh verify example.ngfwlic
# Optional restrictions:
tools/license/generate-license.sh issue --customer "Example Ltd" --days 30 \
  --features ipsec,ha --limit ipsecTunnels=50 --serial "CUSTOMER-SERIAL" --out restricted.ngfwlic
```

Change the defaults at the top of the script or set `NGFW_SIGNING_KEY_DIR`,
`NGFW_SIGNING_PRIVATE_KEY`, `NGFW_SIGNING_PUBLIC_KEY`, `NGFW_LICENSE_DAYS`, or
`NGFW_LICENSE_FEATURES`. Keep the script beside `ngfw-license.mjs`.
The `builtin-public-key` command prints the current placeholder for reference;
its discarded private half cannot be recovered. Newly generated keys are trusted
only after configuring the API with their public key. Private keys stay on the
signing workstation; `api-env` prints only the public key.
