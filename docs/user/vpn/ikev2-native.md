# Native route-based IKEv2

IPsec uses native VPP IKEv2 with a protected IPIP interface. See [IPsec configuration, state and limitations](ipsec.md) for the current product contract. There is one product engine, `vpp-ikev2`; strongSwan is used only as an interoperability peer.

The following excerpt shows the lab responder shape. Replace the interface name and addresses with the underlay interface reported by your appliance, provision `psk/site` through the secrets API, and add the local LAN interface separately. The overlay route selects the protected tunnel by its logical configuration name `site`.

```json
{
  "interfaces": {
    "host-w8nwan": {"enabled": true, "ipv4": ["198.18.8.1/24"]}
  },
  "tunnels": {
    "ipip": {
      "site": {
        "instance": 8001,
        "src": "198.18.8.1", "dst": "198.18.8.2",
        "underlayVrf": "default", "vrf": "default",
        "ipv4": ["198.18.83.1/30"]
      }
    }
  },
  "routing": {
    "static": [{"prefix": "198.18.82.0/24", "vrf": "default",
      "nextHops": [{"address": "198.18.83.2", "interface": "site"}]}]
  },
  "vpn": {
    "ipsec": {
      "proposals": {
        "site": {
          "ike": {"encr": "aes128", "integ": "sha256", "prf": "prfsha256", "dh": "modp2048"},
          "esp": {"encr": "aes128gcm16"}
        }
      },
      "tunnels": {
        "site": {
          "engine": "vpp-ikev2", "ikeVersion": 2, "mode": "tunnel", "protocol": "esp",
          "localAddr": "198.18.8.1", "remoteAddr": "198.18.8.2",
          "localId": "@local.test", "remoteId": "@remote.test",
          "auth": {"method": "psk", "secretRef": "psk/site"},
          "proposal": "site", "routeBased": {"ipipInterface": "site"},
          "localTs": ["198.18.81.0/24"], "remoteTs": ["198.18.82.0/24"],
          "underlayVrf": "default", "vrf": "default", "startAction": "none"
        }
      }
    }
  }
}
```

The IPIP instance must lie in the appliance's allocated VPP ID range. MTU may be omitted; default IPIP MTU read-back is reconciled. FQDN identities avoid the native API's embedded-zero-byte restriction on IP identities.

Runtime CLI commands use the API and require the same authorization as the web controls:

```text
show ipsec sa site
ipsec initiate site
ipsec rekey site 0x12345678
ipsec delete-sa site 0x123456789abcdef0
```

Use SPI values from current state. Local CHILD rekey requires the appliance to be the original IKE initiator. For an original responder, request rekey at the peer. Deleting an SA lowers the protected interface even if its overlay route remains. Creation never raises it before a protected SA exists.

The required patched plugin was tested in disposable VPP instances. It has not been deployed to the shared appliance. [Reproduction instructions](../../../test/topology/ipsec/README.md) distinguish the production agent path from descriptor-only fixtures.
