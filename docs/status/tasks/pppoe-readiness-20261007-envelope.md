# PPPoE readiness task envelope

Branch: codex/pppoe-readiness-20261007
Worktree: /root/ngfw-wt/pppoe-readiness-20261007
Base: f6ae6e555
Owner: /root/pppoe_developer
Owned: PPPoE agent descriptor/renderer/subsystem source/tests; PPPoE dependency lines in deploy/debian/ngfw/debian/control; PPPoE docs; this task status files.
Not owned: RA, installer scripts, tools/lab, board, VPP C/plugin/binapi. No host package/service/NIC mutations.
Scope: recover reviewed checkpoints, repair proven host-independent readiness defects, focused tests only. Owner waived fresh aggregate CI. Real discovery/transit acceptance remains explicit.

Necessary scoped extension: PPPoE-only regression hunk in deploy/debian/ngfw/tests/test_packaging.py, notified manager; no installer code.
