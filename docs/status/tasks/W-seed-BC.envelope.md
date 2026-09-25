# TASK ENVELOPE — W-seed-BC wave-B/C anchor pass (D-119 M1)
id: W-seed-BC   branch: task/W-seed-BC   worktree: /root/ngfw-wt/W-seed-BC   base: task/W-seed@df67a8e (SPECULATIVE, D-114)   started: 2026-09-24T19:15
slot: none — no host runs; unit tests + CI only
read first: /root/ngfw/docs/status/wave-BC-numbers.md (every wave-BC site, the placement rule: seed DIRECTLY ABOVE the first `wave-A:` anchor), /root/ngfw/docs/status/wave-BC-launch-queue.md (M1), /root/ngfw/docs/status/wave-A-hotspots.md, /root/ngfw/prompts/tech-debt/W-seed.md (the same method W-seed used).
scope: anchors only — `// wave-BC: <task-id>` lines at every site in wave-BC-numbers.md incl. the A4 Action cases (det44 ×2, ikev2_sa, remote_access_disconnect, ha_sync, capture, upgrade/support_bundle), SY1–SY5, and the WEB-2 router/nav anchors (`// web: WEB-2`) and the UI-domain-editor W1/W3 anchors (`// wave-A: UI-domain-editor`). The ED nat.go/natTabs EI+CGNAT groups are seeded by F-nat44-ed-sessions itself — do NOT touch nat.go/natTabs. No behaviour change: generated files regenerate byte-identical; same test counts as the base.
files you own: the hotspot files (anchor lines only) · docs/status/tasks/W-seed-BC*
CI: `TMPDIR=/tmp/g-wsbc tools/ci.sh --base main`.
GIT RULE: git only as `git -C /root/ngfw-wt/W-seed-BC …`; NEVER in /root/ngfw.
time box: 3 h. finish: W-seed-BC.md with the per-file anchor list + pasted gen/test/CI evidence; commit; final message = 6 lines.
never: merge · feature logic · pkill
