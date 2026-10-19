# Appliance 211 initial Layer 2 test — agent validation repair required

Branch: `codex/route-ethernet9-11-211-20261019`.
Base local and observed GitHub main: `cb148aafb288301f9fe18eb064a4fc9610ad1698`.
Local task SHA: obtain with `git rev-parse HEAD`; this initial record is part of its own checkpoint.
Remote task SHA/publication: not yet published at initial record creation; publish immediately after the static check and commit, then verify with `git ls-remote`.
Owned files: this status and its adjacent task envelope. No product code modified.

## Observed on 2026-10-19

- HTTPS `/interfaces` returns HTTP 200. Existing SSH key authenticates as root.
- ngfw-api, ngfw-agent and vpp are active.
- Management enp4s0 remains `172.30.110.211/24`.
- Port 9: enp3s0f0np0, VPP index 9, PCI 0000:03:00.0, MAC 28:b1:33:20:94:bb; admin/link up at 10 Gbps.
- Port 11: enp3s0f2np2, VPP index 11, PCI 0000:03:00.2, MAC 28:b1:33:20:94:bd; admin/link up at 10 Gbps.
- VPP has no IP addresses on either port, no bridge-domain, and only default IPv4 drop entries. Physical MTU is 9000.
- Read-only PostgreSQL inspection of network sections: latest revision 3 has 17 enabled physical dataplane ports in default VRF, all with empty IPv4/IPv6 lists. Static routes and ACL attachments are empty; NAT is not configured on these ports.
- Candidate is empty and unlocked (owner/base/payload null). No pending user changes were discarded.
- `ngfw -json show interfaces` exits 3 with not-logged-in; no conventional root CLI session or connected document/browser session was available. No login guesses, credential reads, auth changes or bypass attempted.
- Independent reviewer reproduced the port/link/routing observations. `show l3xc` and IP neighbors are empty; ip4-not-enabled/ip6-not-enabled are present. Although interface-only L3 cross-connect is schema-supported, it does not itself establish IP/ARP readiness, so it is not treated as proven routing.
- GitHub startup inspection: main CI successful for the base SHA; six open PRs (#244–249) belong to other work. Board inspected; no unrelated work adopted or modified. The reviewer is the only observed worker for this task; no claim is made about other workers' liveness.

## User steering and validation result

The user requested Layer 2 first for an initial test, followed by Layer 3, and supplied a normal administrator login. HTTPS authentication succeeded. Full authenticated interface inventory confirms PCI-based Ethernet9 and Ethernet11 labels match enp3s0f0np0 and enp3s0f2np2. Before staging, authenticated diff was empty at revision 3 and lock was unowned.

Staged only reciprocal `routing.l2.xconnects` through HTTPS PATCH. Independent review approves the two-entry patch and confirms VPP automatically enables promiscuous acceptance for L2. No interface, management, IP, NAT or ACL leaves were changed.

POST `/config/validate` failed HTTP400, tier agent, `/interfaces`, rule `basepolicy.namespace`: `cannot establish effective LCP namespace` because binary readback contains control characters while read-only `show lcp` explicitly says `<unset>`. No commit attempted and no traffic-path change applied. API observation separately reports `retrieve pppoe.carrier.namespace: PPPoE kernel helper operation failed`.

PR249 (`44714075332aa75408924c524ae4a5d0f9e90aa5`) contains a reviewed strict read-only namespace corroboration fix, deployed successfully on a different appliance (172.30.126.250) from an older source base. It cannot be blindly copied here. Appliance211 installed package/source is `0.1.0~dev+git695.cb148aafb288`, matching this task's main base. Preserve that provenance when porting the correction. No VPP source/startup, privilege or sandbox changes are authorized/needed.

## Remaining work and next action

Only the candidate has changed; running revision 3 and VPP forwarding are unchanged. Initial Layer 2 and later Layer 3 packet acceptance remain incomplete.

Layer 3 addressing remains unspecified. Proposed lab prefixes were not accepted; the user chose Layer 2 first. Authentication is resolved through the ordinary HTTPS login. A short-lived session and read-only configuration backups are held root-private under `/root/.cache/ngfw-route9-11-211`; never publish their contents.

Next: port the exact reviewed namespace correction to an isolated branch based on installed main; test/review before any surgical agent deployment. Investigate the PPPoE retrieval error read-only. Then repeat ordinary candidate validation, confirmed commit, runtime reciprocal `show mode`, promiscuity, counters and endpoint packet tests. Remove both L2 cross-connects before later L3 configuration.

Exact next read-only command with the current root-private API helper (session may need normal renewal):

```sh
python3 /root/.cache/ngfw-route9-11-211/api.py GET /config/diff
```

Initial `tools/ci.sh check --base origin/main`: PASS (14 seconds). No product changes are included in this operational checkpoint. Product repair tests will be recorded separately. No merge is requested or performed.
