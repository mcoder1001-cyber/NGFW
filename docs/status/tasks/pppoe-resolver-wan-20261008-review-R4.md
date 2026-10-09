# R4 narrow resolver and WAN ownership review

APPROVE only the reviewed resolver preparation and PPP automatic-default
suppression scope at `8ca7d4e251ea1166b96a5ad9e5c03acbb504478a`, tree
`f42fc73d14e12d7950e5cbfcc9da28afbc8cd2e6`.

The fixed unit companion at helper commit
`d43711632d417ee49dac44bcb41e74a5030e19e8` adds only the session resolver output
file as a writable bind within the otherwise read-only PPP directory. Runtime
keeps the mount destination empty and immutable. It prepares the output after
stopping the old session, opens with O_NOFOLLOW and without O_TRUNC, rejects
nonregular/shared-inode files, sets mode 0600 and truncates only after validation.
The symlink/hard-link tests preserve unrelated data. Runtime-owned ancestors and
stopped old dialer remain prerequisites for race-free bind preparation.

The descriptor suppresses rendered Session.DefaultRoute whenever WAN membership
owns the logical PPP interface, without changing operator configuration. Leaving
the group restores the configured behavior. A changed session causes runtime to
withdraw readiness and the old mirrored state before stopping/restarting or
writing new files. Existing WAN reference-context approval remains applicable.

Independent focused race controls for resolver file safety, installed writable
scope and default ownership passed subsystems in 1.081 seconds and PPP descriptor
in 1.029 seconds. Companion helper loader passed 28 controls in 0.013 seconds.
No CI, daemon, route or packet activation was performed.

This is not cumulative approval of the entire carrier implementation. Combined
health loss/recovery, join/leave, stale observations and rollback must still prove
that no automatic default bypasses WAN health selection. The separately reported
scheduler TAP-loss/recreation issue is outside this narrow review and requires
its own correction and independent recheck. Native resolver mount/DNS behavior,
kernel/VPP readiness and forwarding acceptance remain deferred.
