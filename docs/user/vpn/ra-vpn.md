# Remote-access VPN

Remote access uses an independent strongSwan IKEv2 engine. Site-to-site native IKEv2 profiles remain separate. Check **VPN → Remote access** before enabling a profile: a nonoperational engine permits editing disabled drafts, but it does not mean clients can connect. Enabling also requires a supported authentication method, explicit transit addressing and existing ingress and egress ACL references for both transit links.

## Create a profile

The wizard writes the candidate configuration. Validate and commit it through the normal configuration workflow after reviewing the changes.

1. Choose the authentication method, dedicated routed public endpoint and server identity. Select the protected VRF and public underlay VRF, an existing server certificate and IKE/ESP proposal. For EAP-TLS or public-key authentication, select the existing client CA.
2. Enter the outer and inner transit addresses, with an optional IPv6 inner transit for IPv6 pools. Choose existing ACLs for **both ingress and egress** of the protected and public transits. The engine adds no implicit allow policy. The public endpoint must not collide with a native site-to-site listener.
3. Add named client pool prefixes and DNS addresses. Set protected split-tunnel routes; an empty list requests a full tunnel. Pool overlap and references are checked by the API.
4. For EAP-MSCHAPv2, add unique usernames and existing `password/<name>` references. For EAP-RADIUS, add server addresses, ports and existing `psk/<name>` shared-secret references. Adjust dead-peer and rekey timers. Passwords and RADIUS secrets are never entered into this configuration form.

An EAP-MSCHAPv2 draft uses `auth: "eap-mschapv2"` and `users: [{ "username": "alice", "passwordRef": "password/alice" }]`. An EAP-TLS draft uses `auth: "eap-tls"` and `clientCa: "employees"`, referring to the existing PKI CA. Both require the server certificate, proposal, pools and explicit transit/ACL settings described above. These fragments are not complete deployable profiles.

## Client setup

Use the configured server identity as the VPN server name, and verify that the server certificate matches it. Distribute the trusted CA through your managed device policy; never bypass certificate verification.

- **Windows:** add an IKEv2 VPN connection. Select the authentication method matching the profile: username and password for EAP-MSCHAPv2, or an installed client certificate and private key for EAP-TLS. Install the trusted server CA and client certificate in the appropriate certificate stores. See the [strongSwan Windows client guide](https://docs.strongswan.org/docs/latest/interop/windowsClients.html).
- **macOS, iOS and iPadOS:** deploy an IKEv2 VPN configuration with the server address, remote identity and matching EAP authentication. Install the managed CA and, for certificate authentication, the client identity. Use device-management profiles where required by the OS. See the [strongSwan Apple client guide](https://docs.strongswan.org/docs/latest/interop/ios.html).
- **Android:** use the device's supported IKEv2 client, or the strongSwan client. Choose the matching EAP method and verify the configured server CA and identity. Device support for each authentication method varies.
- **strongSwan:** configure IKEv2 to the public endpoint with the expected server identity and trusted CA; request a virtual address and select `eap-mschapv2` or `eap-tls` for the local authentication. Keep passwords and private keys in the client's protected credential store. Accept only the authorized pool DNS and split routes.

Client-specific screens vary by OS release. A certificate must chain to the configured client CA and satisfy the engine's certificate validity and revocation checks. A foreign or revoked certificate must be refused.

## Observe and disconnect

Select a profile to view the engine's observed sessions. The grid displays identity, assigned addresses, uptime and exact byte counters. Empty results mean no sessions were observed for that profile. A failed request is shown as an error, not as a healthy empty list. Pagination uses an opaque cursor and a maximum of 100 sessions per request.

Administrators can disconnect a session after confirmation. Operators and read-only users can observe sessions but cannot disconnect them. Success requires the agent to remove the owned session and verify its absence. A stale or foreign session, unverified removal or runtime failure is reported as a failure and audited.

## API and command-line equivalents

Use the normal authenticated configuration pointer routes for `vpn.remoteAccess.<name>`; validate and commit the candidate through the existing configuration commands. The wizard does not activate a profile directly. For an existing, fully configured `office` profile, the CLI can stage a disabled draft and inspect it:

```text
ngfw merge vpn remoteAccess office '{"enabled":false}'
ngfw configure show vpn remoteAccess office
ngfw validate
```

Switch that existing draft to EAP-TLS with `ngfw set vpn remoteAccess office auth eap-tls` and `ngfw set vpn remoteAccess office clientCa employees`. For EAP-MSCHAPv2, merge `{"auth":"eap-mschapv2","users":[{"username":"alice","passwordRef":"password/alice"}]}` into the same profile. The certificate, pools, transport and both policies must already be configured. Validate and review the diff before a confirmed commit. See the [CLI reference](../cli/reference.md) for credential-file options and commit confirmation commands.

For read-only observations and the administrator action, an authenticated CLI HTTP client can use:

```text
GET /api/v1/state/vpn/remote-access/capabilities
GET /api/v1/state/vpn/remote-access/sessions?profile=office&limit=50
GET /api/v1/state/vpn/remote-access/sessions?profile=office&limit=50&cursor=<opaque-cursor>
POST /api/v1/actions/vpn/remote-access/sessions/<opaque-session-id>/disconnect?profile=office
```

Session IDs are opaque 64-character lowercase hexadecimal values. Counters are decimal strings, preserving the full 64-bit range. No namespace, process identifier, internal interface index, password or RADIUS shared secret is exposed by these routes.
