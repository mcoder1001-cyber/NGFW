# ip6_nd descriptors (DF-2, WBS D2.3: RA, proxy-ND, DAD)

Package `apps/agent/internal/descriptors/ip6_nd` (Go package `ip6nd`). Messages from `apps/agent/binapi/ip6_nd` and `binapi/ip6_dad`.

| Object | Descriptor / key | Create / Update / Delete | Retrieve | Dependencies | Notes |
|---|---|---|---|---|---|
| RA config | `ip6-nd.ra-config` / `ip6-nd.ra-config/<ifname>` | `sw_interface_ip6nd_ra_config` (suppress, managed, other, ll_option, send_unicast, cease, default_router via lifetime, lifetime, initial count/interval, max/min interval); Update in place; Delete restores the fresh-interface state | `sw_interface_ip6nd_ra_dump` (all flags and timers decoded); interfaces in the default state are omitted | `interface/<ifname>` | Desired values must be passed through `NormalizeRaConfig` (zero timers → VPP defaults 600/200/150/3/16, min = 0.75 × max) or the diff never converges. |
| RA prefix | `ip6-nd.ra-prefix` / `ip6-nd.ra-prefix/<ifname>/<prefix>` | `sw_interface_ip6nd_ra_prefix` (valid/preferred lifetime, no_advertise, off_link, no_autoconfig; `no_onlink` is not modelled: the dump has a single `onlink_flag`, so off_link and no_onlink cannot be told apart and only `off_link` round-trips); Update in place; Delete with `is_no` | `sw_interface_ip6nd_ra_dump` → `prefixes` | `interface/<ifname>`, `interface-ip/<ifname>/<prefix>` (Optional) | `NormalizeRaPrefix` canonicalises the prefix. |
| proxy ND | `ip6-nd.proxy` / `ip6-nd.proxy/<ifname>/<ip6>` | `ip6nd_proxy_enable_disable` + `ip6nd_proxy_add_del`; Update → `ErrRecreate` | `ip6nd_proxy_dump` | `interface/<ifname>` | **Host integration test disabled by default** (`VRX_DF2_PROXY_ND=1` to run): the first `ip6nd_proxy_add_del` on vrx-a crashed VPP 26.06 on 2026-09-23 15:52 — see DF-2-questions.md. Unit-tested on the fake. |
| DAD | `ip6-nd.dad` / `ip6-nd.dad/global` | `ip6_dad_enable_disable` (transmits, retransmit delay); Update = re-enable with new values; Delete = disable | `ip6_dad_dump` | none | Global switch. On 26.06 the `ip6_dad` API is core (vnet), so it works on vrx-a although `ip6_dad_autoremove` is not loaded; if a VPP lacks the messages every method returns `df2.ErrPluginNotLoaded` and the integration test skips (skip-unless-plugin-loaded). |

`ip6nd_send_router_solicitation` is an action (not built). CLI equivalent: `show ip6 interface <if>`.
