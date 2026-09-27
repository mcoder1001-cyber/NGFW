# Task: F-setup-wizard — First-boot setup wizard   (prepend 00-CONTEXT.md)

## Goal
A new box is usable in five minutes without knowing the data model: on first login (factory state) the UI runs a wizard
instead of the dashboard.

## Inputs to read first
- `F-system-identity`, `F-management-ui`, P07b (login, pending-change bar, commit dialog), `F-kea-dhcp-relay`,
  `F-nat44-ed-sessions`, `F-host-acl-nftables`, `F-pppoe-client` (optional step when merged).

## Contract changes
`system.setup { completed: bool, completedAt }` (read-only in the normal editor). No other new config: every step writes the
existing domains.

## Scope — build exactly this
1. **Steps**: (1) language + time zone + NTP; (2) admin password change (mandatory, policy-checked); (3) hostname;
   (4) WAN: pick interface, DHCP / static / PPPoE (PPPoE only if F-pppoe-client is merged); (5) LAN: interface, address,
   DHCP server on/off with a suggested pool; (6) defaults: outbound NAT on WAN, LAN→WAN allow, block inbound, management
   only from LAN; (7) summary showing the exact config diff → one commit (with confirm timer).
2. Wizard state is kept client-side until the final commit; nothing is applied step by step. Back/next, validation per step
   through the normal schema validation endpoint.
3. Re-runnable from System → "Run setup wizard" (warns that it overwrites the touched sections).
4. **Tests**: unit tests for the diff builder; Playwright run through all steps on a factory-state API; screenshot of each step.
5. **Docs**: `docs/user/getting-started.md`.

## Acceptance (paste the evidence)
- [ ] Factory box → wizard → LAN client gets DHCP and reaches the Internet through NAT on the rig (pasted)
- [ ] Management reachable from LAN only after the wizard (pasted)
- [ ] Screenshots of every step, fa (RTL) and en
- [ ] `tools/ci.sh --base main` green

## Out of scope
Multi-WAN, VPN, HA setup.
