# Setup interface list repair

Branch: `codex/fix-setup-interface-list`, based on origin/main `2955461ea`.
Owned files: `deploy/systemd/ngfw-api.service`, this status file.

The appliance unit omitted the documented D-192 `NGFW_SEED_DEFAULT_NICS=1` opt-in. On 172.30.110.211 the agent returned all 18 physical NICs, with enp4s0 identified as management, but all rows were inventory-only. Setup correctly excludes such rows from WAN selection.

Change: enable default NIC registration in the packaged appliance API unit. The API environment file can still override it; development's schema default stays disabled.

Validation: `python3 deploy/debian/ngfw/tests/test_firstboot.py`: 7 tests passed. Independent read-only review found no actionable issues. A persistent systemd drop-in was installed on 211 and only ngfw-api restarted. API observations remain available with no observation errors; management remains unconfigured as a dataplane interface.

Resolved: the user removed the pending BondEthernet0 change. The automatic retry then committed revision 1 with 17 data NICs. `/api/v1/state/interfaces` confirms all 17 physical dataplane configuration rows and no observation errors. enp4s0 remains inventory-only with no running interface configuration. ngfw-api, ngfw-agent and vpp are active. No data NICs are live in VPP yet; dataplane binding/traffic readiness is separate from list registration. No VPP restart was performed.

Publication: remote branch `codex/fix-setup-interface-list` contains code commit `a45f82100`. Static CI check passed; 15 focused SetupWizardPage, setup regression and DashboardOverview tests passed. Draft PR: https://github.com/mcoder1001-cyber/NGFW/pull/234. Complete quick gate result pending.
