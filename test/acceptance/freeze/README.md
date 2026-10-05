# Freeze acceptance

From the repository root:

```sh
python3 test/acceptance/freeze/run.py --output .scratch/freeze-plan
python3 test/acceptance/freeze/run.py --run-offline --output .scratch/freeze-offline
```

Run after the complete quick gate, with no build/generator running in the same
worktree. The contract build uses Turbo's existing dependency ordering and may
regenerate committed outputs; inspect `git status` afterward.

The runner executes existing cross-component reachability, commit/rollback and
authentication regressions through the heavy-step scheduler. It records the source
SHA and dirty-tree flag, retains failures, and exits nonzero on failed checks.
Raw logs remain private and must be reviewed/redacted before publication.

The generated manifest separately lists unexecuted forwarding, wave B/C, browser
and appliance cases. Passing offline cases does **not** set `release_accepted`.
Execute those cases using their existing owned-slot procedures and collect exact
source, command, expected/actual result and cleanup evidence in
`docs/status/DEFERRED-ACCEPTANCE.md`. Never restart the shared VPP for this campaign.

Native VPP route-based IKEv2 is the current IPsec implementation. Historical
strongSwan/kernel-vpp product packaging is superseded by
`docs/decisions/DEC-ipsec-route-based.md`; strongSwan remains an optional test peer.
