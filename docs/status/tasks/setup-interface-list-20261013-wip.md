# Setup interface list repair

Branch: `codex/fix-setup-interface-list`, based on origin/main `2955461ea`.
Owned files: `deploy/systemd/ngfw-api.service`, this status file.

The appliance unit omitted the documented D-192 `NGFW_SEED_DEFAULT_NICS=1` opt-in. On 172.30.110.211 the agent returned all 18 physical NICs, with enp4s0 identified as management, but all rows were inventory-only. Setup correctly excludes such rows from WAN selection.

Change: enable default NIC registration in the packaged appliance API unit. The API environment file can still override it; development's schema default stays disabled.

Validation: `python3 deploy/debian/ngfw/tests/test_firstboot.py`: 7 tests passed. Independent read-only review found no actionable issues. A persistent systemd drop-in was installed on 211 and only ngfw-api restarted. API observations remain available with no observation errors; management remains unconfigured as a dataplane interface.

Current blocker: the device has an uncommitted candidate addition at `/interfaces/BondEthernet0`, owned by admin. SeedService reports `seed deferred (the candidate has uncommitted changes); will retry`. User was asked whether to preserve or discard that change; no approval is assumed. No candidate edits have been discarded. Interface registration/selector restoration is not yet verified.

Next action: honor the user's candidate decision, allow the seed retry, and verify configured data rows and exclusion of enp4s0. If preserving the candidate, registration remains deferred until the owner resolves it.
