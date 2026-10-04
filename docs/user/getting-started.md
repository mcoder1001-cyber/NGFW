# First-boot setup

Sign in as the local administrator over HTTPS. A device whose setup marker is incomplete opens the setup wizard from the dashboard. The wizard is also available at **System → Run setup wizard**. Re-running requires acknowledging that the touched configuration sections will be overwritten.

1. Choose English or Persian, an IANA time zone, and NTP servers.
2. Enter the current administrator password and a different new password of at least 12 characters. Passwords remain in this page's memory; they are never saved in browser storage or included in the summary.
3. Choose the hostname.
4. Select an existing WAN interface. Choose DHCP or static IPv4 with a gateway. PPPoE is unavailable until its client implementation is integrated; unsupported mode requests are rejected by the shared schema.
5. Select a different existing LAN interface and its IPv4 address/prefix. Prefixes /8 through /29 are supported. Enable DHCP if needed; the suggested pool excludes the network, broadcast, and router address and contains at most 200 addresses.
6. Review safe defaults: outbound NAT on WAN, stateful LAN access, unsolicited inbound blocking, management from the LAN prefix and Linux tap only, with anti-lockout enabled. Required DHCP traffic remains allowed.
7. Review the exact diff. Nothing is applied during Back/Next. Apply stages all settings and the administrator password together, validates the candidate through the existing engine, and makes one confirmed commit with a 120-second timer.

Reconnect through LAN, check management access, and select **Confirm LAN management works** before the displayed deadline. Otherwise the configuration reverts. Confirmation promotes the new password and invalidates old credentials; sign in again using the new password. If validation or commit fails, the candidate remains available for inspection or discard through the normal configuration controls. The current running configuration and administrator password have not been promoted by staging.

The wizard preserves unrelated configuration, but overwrites addressing on the selected interfaces, NAT44 inside/outside/pool settings, input ACL attachments on those interfaces, host input attachments, time/identity settings and its own `setup-lan` DHCP instance. Existing objects remain in the document when they are no longer attached. Completion metadata is read-only through ordinary config edits. An existing candidate must be committed or discarded first, and a changed running revision requires reloading the wizard.

API/CLI equivalent: use authenticated `POST /api/v1/config/setup/preview` with `input`, `baseRevision` (the running `x-ngfw-revision`), and a current ISO `completedAt`; stage with the same body plus write-only `current` and `password` through `/setup/stage`; then call `/api/v1/config/validate`, `/api/v1/config/commit?confirm=120`, and `/api/v1/config/commit/confirm`. Password-bearing requests require HTTPS or a local transport. Never pass passwords as shell command arguments or record them in command history.

Live DHCP lease, Internet packet delivery, LAN-only reachability and screenshot acceptance require an isolated appliance/rig. Unit verification does not establish those hardware results.

For real-browser seven-step English/Persian preview screenshots without applying configuration, run `apps/web/test/e2e/screens/setup.mjs` against a factory-state API with `NGFW_E2E_BASE`, `NGFW_E2E_SETUP_WAN`, `NGFW_E2E_SETUP_LAN`, `NGFW_E2E_ADMIN_PASSWORD_FILE`, and the existing Playwright/Chrome paths. This exercises server preview but deliberately stops before Apply; packet and confirmed-commit acceptance on the isolated rig remains separate.
