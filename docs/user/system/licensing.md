# Licensing

VRX uses a simple, **offline** licence: a signed file that says who the licence is for, how long it is valid and which
features it covers. There is no online activation and no call-home. Licensing never touches the data plane: it is
checked only when you commit a configuration.

## What happens when

| Situation | Status | Effect |
|---|---|---|
| No licence installed | `community` | The community feature set applies (table below). |
| Licence valid | `valid` | The licence's features and limits apply. |
| Licence expired, less than 30 days ago | `grace` | Licence still applies; every commit returns a warning; a banner is shown; a `license.grace` system event is recorded. |
| Licence expired more than 30 days ago | `expired` | Community set for **new** configuration; a `license.expired` system event is recorded. |
| Wrong host (binding), not valid yet, or the stored file does not verify | `invalid` | Community set for new configuration. |

**Running configuration is never removed.** When a licence expires, everything already committed stays applied and keeps
forwarding. Only a commit that *adds* an unlicensed feature (a node that is not in the running configuration) is rejected
with `403` `application/problem+json`, `type …/license-required`, `tier: "license"` and `errors[].pointer` naming the
first offending node, for example `/ha/vrrp/lan-v4`.

## Entitlement table (SAMPLE — to be confirmed by the product owner)

| Feature id | Used when the configuration has | Community |
|---|---|---|
| `ipsec` | `/vpn/ipsec/tunnels/*` | no |
| `wireguard` | `/vpn/wireguard/interfaces/*` | yes, up to 2 interfaces |
| `bgp` | `/routing/bgp` | no |
| `ospf` | `/routing/ospf` | yes |
| `isis` | `/routing/isis` | no |
| `ha` | `/ha/vrrp/*`, `/ha/cluster` | no |

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
- `PUT /api/v1/system/license` (admin, audited): body = the `.vrxlic` file content. A malformed, tampered or wrongly
  signed file, a file bound to another host, and a file expired past grace are rejected with `400`.
- CLI (`vrx`): the generated operations for these two routes (see `docs/user/cli/reference.md`). The CLI enforces
  nothing itself; the API does.

## File format (`.vrxlic`)

```json
{
  "format": "vrxlic/1",
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

The API trusts the product public key(s) compiled into `apps/api/src/features/licensing/licensing.config.ts`. For
development only, `VRX_LICENSE_PUBKEY_FILE` adds one more trusted key. The installed file is stored at
`VRX_LICENSE_FILE` (default `/var/lib/vrx/license.vrxlic`). `VRX_LICENSE_SERIAL` overrides the detected serial.

## How support issues a licence

`tools/license/vrx-license` needs only Node 22 (no packages):

```sh
# once, on the offline signing workstation — never inside a git checkout (the tool refuses)
tools/license/vrx-license keygen --out-dir /secure/vrx-license-keys
# → vrx-license-signing.pem (0600, keep offline) and vrx-license-public.pem (goes into licensing.config.ts)

# the customer sends the serial:  cat /sys/class/dmi/id/product_serial
tools/license/vrx-license issue --key /secure/vrx-license-keys/vrx-license-signing.pem \
  --customer "Example Ltd" --id LIC-2026-0001 --days 365 --serial "VMware-42 1a 2b ..." \
  --features ipsec,ha --limit ipsecTunnels=50 --out example.vrxlic

tools/license/vrx-license verify --pub /secure/vrx-license-keys/vrx-license-public.pem example.vrxlic
tools/license/vrx-license inspect example.vrxlic
```

The customer uploads `example.vrxlic` on **System → Licence**.
