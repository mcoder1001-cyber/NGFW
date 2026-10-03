# V27 — LCP multicast lifecycle repair (2026-10-03)

The RIP restart fixture recreated two default-namespace LCP pairs with correct Linux/VPP IPv4 addresses and RIP multicast membership. Only the WAN physical interface retained plugin-low Accept for `224.0.0.0/24`; the LAN physical interface did not. Core SPECIAL Accept for `224.0.0.1/32` and `224.0.0.2/32` remained on both interfaces, isolating the Linux CP multicast lifecycle rather than IPv4 enablement.

Pinned `lcp_router.c` registered only a pair-delete callback, which removed IPv6 but omitted IPv4. It added multicast inputs through Linux address notifications. Pair recreation with existing addresses or configuration through VPP could therefore depend on netlink event timing.

[Patch 0003](../../../deploy/vpp/patches/0003-lcp-multicast-reconcile.patch) reconciles plugin-low IPv4 Accept on pair creation and VPP address changes, removes IPv4/IPv6 inputs symmetrically on pair deletion, and uses IPv6 link delegates to cover existing or newly enabled link-local state and final disable. The Linux IPv6 notification handler now reconciles the completed link state so deleting the last global address does not erase acceptance for a retained link-local reference. The delegate includes a formatter required by `show ip6 interface`.

The patch changes `linux_nl_plugin.so`; the router source belongs to that module. It adds no API-source multicast route and modifies only the affected physical-interface path within the existing plugin-low source.

Validation:

- [Stock reproduction](lcp-multicast-2026-10-03-evidence/stock-reproduction.log): the dedicated fixture fails because plugin-low `224/24` acceptance is absent, while both core SPECIAL inputs remain.
- [Private build](lcp-multicast-2026-10-03-evidence/private-build.log): copied router source compiled and linked using existing pinned build objects. Compiler and linker dependency outputs are redirected/removed; upstream source and objects are unchanged.
- [Patched lifecycle proof](lcp-multicast-2026-10-03-evidence/patched-lifecycle.log): two pairs, recreation with existing addresses, multiple IPv4 addresses, final address removal, restoration, independent other-pair preservation, IPv6 link-local-only recreation, interface formatting, direct and real Linux-netlink global address deletion retaining link-local acceptance, final IPv6 disable and symmetric pair deletion all passed.

Reproduction uses `test/topology/build-lcp-multicast-plugin.py` and `test/topology/lcp-multicast-reconcile.py` through the disposable VPP wrapper inside a private network namespace. Both patched and stock disposable VPP processes stopped. The shared VPP was not reconfigured or restarted. Patch is registered as product in the ordinary series, version `26.06-release+vrx3`, after independent review and strict routing acceptance. No package/plugin was installed on the shared appliance.

Strict full ISIS/RIP production-agent acceptance passed in 154.20s: agent restart recovery 2.8s, post-restart LAN withdrawal 300ms, reannouncement 800ms restored all 40 FRR/VPP routes, rollback removed all routes in 300ms and Retrieve/cleanup were empty. See [routing proof](F-isis-rip-host-2026-10-03.md). The earlier stock and fixture failures remain recorded; diagnostic skips are not counted as passing acceptance.

Product deployment static gate: `deploy/vpp/verify.sh` PASS, including 66/66 packaging/series tests; [output](lcp-multicast-2026-10-03-evidence/product-verify.txt). Signed package builds/install acceptance remain distinct release work.

Provenance: routing build hashes capture the exact test-time patch. After acceptance only its Track/Status metadata changed to V27/product, and the helper description was updated; the compiled C patch body is unchanged. [Registered source hashes](lcp-multicast-2026-10-03-evidence/registered-source.sha256) identify the final series files.
