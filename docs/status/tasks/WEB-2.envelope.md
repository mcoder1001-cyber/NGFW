# TASK ENVELOPE — WEB-2 config screen kit + data widgets + Secrets page (web-ahead track, D-123)
id: WEB-2   branch: task/WEB-2   worktree: /root/ngfw-wt/WEB-2   base: task/P08@abb6950 (SPECULATIVE D-114: you extract P08's generic half; P08 is approved-pending-merge)   started: 2026-09-24T18:08
slot: 1 → eval "$(tools/lab env 1)" (only for a dev server / e2e; unit tests need no slot)
scope: a reusable config-screen kit in apps/web/src/config/{collection,widgets}/** — generic list + drawer + live-status slot over any candidate domain path (the pattern P08's interfaces screen and System › Users use), data widgets (ServerDataGrid wrappers, status chips, counters); a Secrets page (list/create/rotate/delete by ref; the secret VALUE never enters form state, query cache or logs; key cell is a real button — keyboard usable, A11Y-1). Router/nav lines only under `// web: WEB-2` anchors if they exist on main at merge; otherwise the Secrets page stays unregistered (no routed stub). Soft dep WEB-1 (use its widgets if merged; never block on it).
files you own: apps/web/src/config/{collection,widgets}/** apps/web/src/pages/SecretsPage{,.test}.tsx apps/web/src/pages/dev/previews.ts apps/web/src/locales/{en,fa}/config.json (kit.* keys only) docs/status/tasks/WEB-2*
est/time box: 9 h / 13 h.
GIT RULE: run git ONLY inside your own worktree (`git -C <worktree> …`); NEVER in /root/ngfw (main — the merger works there).
CI: `TMPDIR=/tmp/g-<id> tools/ci.sh --base main` (short TMPDIR). Ports 3000/8080/9101 and /run/vrx/agent.sock belong to tools/app — never touch them. Stagger heavy test runs (host load; D-121).
Architecture (00-CONTEXT) is non-negotiable: one schema → three consumers (never hand-duplicate a type; UI derives from packages/schema); every UI string through t() with identical en/fa keys; logical CSS only; no UI screen with a stubbed backend is routed; secrets never enter form state, query cache or logs.
WIP commits every 45 min; finish with docs/status/tasks/<id>.md (what / how verified with pasted output / out of scope / questions), all committed, final message = 8-line summary.
never: merge · edit files you do not own · restart/kill VPP · pkill
