# Task envelope — apply-211-vpp-startup-20261020

Manager step: install the permanent VPP start-up file on appliance 172.30.110.211, rendered from the dataplane document
of task machine-211-vpp-tuning (PR #251), through the product apply tool, after the owner's confirmation of the outage.

Started by the owner's session (`ماشین 110.211 ترافیک`) on 2026-10-20. Base origin/main 5593e5ab2. Branch
`claude/magical-cartwright-cda9e2`, worktree `/root/NGFW/.claude/worktrees/magical-cartwright-cda9e2`.

Owned files: `docs/decisions/PENDING-211-vpp-startup-tuning.md` (renamed `DEC-256-211-vpp-startup-tuning.md` after the apply), `docs/status/tasks/apply-211-vpp-startup-20261020*`,
the D-256 row of `docs/decisions/LOG.md`, the board row `apply-211-vpp-startup-20261020`. Not owned:
`docs/status/tasks/machine-211-vpp-tuning*` (PR #251; a pointer is added there after that PR merges).

Host 172.30.110.211: the dataplane document copied to `/root/machine-211-vpp-tuning-dataplane.json`; read-only checks;
`/etc/ngfw/dataplane-performance.json` + one run of `/usr/local/libexec/ngfw-dataplane-performance`; one apply through
`/usr/lib/ngfw/apply-startup.sh --mode product` (which restarts VPP once). No hand edit of `/etc/vpp/startup.conf`, no
VPP restart outside the tool, no `systemctl restart` of any unit `vpp.service` requires.

Acceptance: the new file live (sha256 `92985d0c…`) with `COMMITTED` from the tool; `show threads` main 16 / workers
2,4,6,8,10,12,14; every X710 TX queue `shared no`; no new Malicious Driver Detection event; the agent resync complete
(17 ports up, three xconnect pairs); under the owner's TRex test no `tx-error` growth on the six test ports; the
evidence in the wip file, D-256 and the board, pushed. The running-configuration commit of the new dataplane domain is
an explicit follow-up, not part of this envelope.
