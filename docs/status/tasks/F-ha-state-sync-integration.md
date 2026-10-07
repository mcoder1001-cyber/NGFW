# HA integration correction

Root-owned integration worktree /root/ngfw-wt/integrate-ha-20261005 on
codex/integrate-ha-20261005, source base33a081fb. The developer's unchanged
complete quick failed the generated-output guard because its committed YANG
schema omitted the new NAT44-EI listener/peer fields. The normal `pnpm gen`
produced the missing40 lines; no generated source was hand-edited and no guard
was weakened. Copy that exact generated artifact into this separate integration
worktree and publish a corrective checkpoint to PR178. No product author tree
is edited by this correction. Rerun the unchanged complete gate here with
TURBO_ENV_MODE=loose, dedicated TMPDIR, bounded concurrency and heavy semaphore.

Old failed gate:
/root/ngfw-wt/logs/ci/ready-f-ha-state-sync-20261005-20261005-070007-1662321.
No old PASS or final merge is claimed. Independent mandatory panels still pending.
