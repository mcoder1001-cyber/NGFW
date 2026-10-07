# Test host management persistence

Branch: codex/test-host-management-20261007
Worktree: /root/ngfw-wt/test-host-management-20261007
Owned files: docs/status/tasks/test-host-management-20261007*, deploy/test-hosts/20261007/*.
Scope: owner-authorized root SSH administration of 172.30.110.211 and 172.30.126.37; preserve management IP/default route/DNS before installation or reboot. No changes to shared development host networking/VPP. Preserve existing target configuration backups. No disk erasure.
Recovery: read WIP; verify SSH and target addresses before every network operation. Publish every coherent checkpoint. Do not claim installation or reboot acceptance without observed evidence.
