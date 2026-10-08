# Setup wizard PPPoE completion

Branch `codex/setup-pppoe-20261008`, base `85a1626f`. Own setup input/builder/tests, API setup controller/tests, web setup component/tests, setup locales and feature documentation only.

Contract: additive WAN mode `pppoe` plus `wanPppoe { username, passwordRef }`, using the existing PPPoE username and password-reference schemas. The request contains no ISP password plaintext. DHCP/static modes reject retained PPPoE credentials. The wizard creates a separate deterministic logical `setup-pppoe` interface with an explicit exclusive physical parent, and attaches NAT/ACL/default-route behavior to the logical WAN. Collisions and modified/non-wizard reuse fail closed. Rerun removes only demonstrably wizard-owned logical state; unrelated references prevent deletion.

Implementation and focused schema/API/web en/fa tests pending. No CI triggered; final combined CI remains manager-owned. Native ISP and LAN acceptance remains laboratory work.
