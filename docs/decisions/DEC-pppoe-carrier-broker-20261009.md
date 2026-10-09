# PPP carrier: fixed broker and isolated dialer

The completion campaign implements the accepted PPPoE client datapath using the
existing VPP API and the packaged kernel PPP daemon. It does not add VPP C code
or activate an appliance service. Source review and the final combined CI remain
required before merge; native forwarding and installed-unit acceptance are separate.

## Boundary and alternatives

The privileged agent retains its existing five-capability unit and writable paths.
The fixed root broker accepts only enumerated operations for a validated carrier
specification. A request is bound to its owner, nonce, boot identity, digest and
short expiry, rechecked after locking. There is no arbitrary command or user path.
The broker needs the host mount namespace so persistent /run/netns mounts are
visible to VPP. Therefore private-mount sandbox switches are not added to that
unit; bounded operations, the capability set, device policy and time limit are
part of its boundary. This is not a claim that the broker has no host privilege.

The dialer starts only with verified namespace and generation identity. Its fixed
helper enters that namespace and drops to NET_ADMIN and NET_RAW before the fixed
pppd executable; no ambient/inheritable privilege is granted. Private peer files
live under /var/lib/ngfw/agent/pppoe-carrier. The unit mounts its PPP directory
read-only with only the per-session resolver output file writable. It exposes
/dev/ppp and its own state directory, not a broad writable /etc.

Alternatives considered: VPP C changes (excluded by the plan); treating a
negotiated PPP peer as a usable physical-interface gateway (not a forwarding
carrier); direct namespace provisioning inside the agent's mount sandbox
(persistent mounts would not be visible to VPP); fixed host broker plus isolated
kernel dialer (selected). The separate raw L2 transport and logical IP transit
keep policy/NAT identity on the configured logical PPP interface. Linux NAT is
not introduced.

The packaged pppd executable and PPPoE plugin are trusted components. Namespace
IP filtering does not contain arbitrary Ethernet injection by a malicious daemon
with NET_RAW. No stronger claim is made. Runtime readiness requires actual kernel
and VPP readback, and short-lived generation-bound evidence; helper fixtures alone
are not native forwarding acceptance.

The choice is reversible by withdrawing the carrier objects and removing its
fixed package assets; it does not widen the main agent unit. Existing independent
helper and resolver reviews describe their exact source scope. Native broker
mount visibility, PPP negotiation, VLAN/QinQ packets, resolver output, VPP policy,
reconnect, delegated LAN addresses/RA and reboot remain NOT RUN on the final
completion candidate until a real appliance campaign records them.
