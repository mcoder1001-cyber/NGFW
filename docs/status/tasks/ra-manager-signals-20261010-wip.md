# RA manager signal repair WIP

Branch `codex/ra-manager-signals-20261010`, worktree `/root/ngfw-wt/ra-manager-signals-20261010`, source base `ded860762c81e72fb9f760caec4bd98898c8b6a4`. First local/remote contract checkpoint `d664b2542`. Owned files are listed in envelope.

Completed pure wire contract: strict type4 framing and closed public Manager/PropertiesChanged header whitelist; original reply validator unchanged. Bodies remain opaque and grant no property or pending-serial authority. Manager, escaped unit, and numeric job paths are primary-source backed; systemd v259.5 dbus-job.c:215 explicitly emits PropertiesChanged on job paths. Original 16KiB connection budget and two-second query deadline remain required. Transport consumer has not changed.

Actual protected PID1 probe during an already-authorized WAN activation observed mixed type4 and type2 on all five direct connections, without Subscribe/Hello or extra unit actions. Original raw-header/count receipts retained; original total 16KiB observation bound stopped each connection. Quiet metadata followup observed replies only, so actual signal metadata has not yet been captured. PID1 identity unchanged. This proves the old transport rejects actual lawful traffic; guest5-specific causation remains unproven.

Focused race pure-wire suite initially passed (1.182s); final verbose rerun follows numeric-job correction. Remaining: independently reviewed consumer, meaningful mixed-frame/rights/budget/orphan/race tests, actual read-only property consumption proof, integrate latest main and unchanged complete quick/hosted gates. Exact next command: publish pure-wire contract after final focused tests, then implement transport discard consumer following manager approval. No full RA operational or Done claim.
