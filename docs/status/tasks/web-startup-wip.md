# Web startup WIP

Branch: codex/ngfw-web-startup
Worktree: /home/mohammad/ngfw-worktrees/web-startup
Owned files: NGFW web migration integration, packages/ui-kit theme, local shared-package staging tool and dependency configuration.
Previous local checkpoint: 76b76c708.
Remote checkpoint: c770004f7d975f14a2f5ff326136c25506f16088 on codex/ngfw-web-startup-checkpoint (published via GitHub connector). New changes are not yet published.

Completed: integrated shared theme, restored interface aliases and raw action identifiers, resolved duplicate merge imports, restored optional WAN setup behavior, narrowed i18n metadata lint exemptions, lazy-loaded Login to reduce entry size. Independent review found spacing must remain a function with NGFW's 8px unit; corrected.
Actual checks before user update: web typecheck passed; web lint passed including logical CSS; browser dashboard rendered at port 5174 with sample data. Full web tests/build not complete; do not claim full CI green.

2026-10-19 user updated /home/mohammad/common-ui. Latest source supersedes earlier recovery; common-ui agent now reassesses that baseline in isolation. Original project source remains untouched by this task. Prior preview processes are no longer listening.
Remaining: build updated shared package, refresh isolated NGFW dependency, rerun typecheck/theme and web tests/production bundle checks, restart preview, browser verification, publish checkpoint. No merge authorized by passing checks yet.
Next command: inspect common-ui agent's new build result, then stage its validated package into .yalc and pnpm install using isolated store.

## Updated common-ui reconsideration

Used updated user source as baseline, with narrow package import fixes in common-ui branch codex/common-ui-updated-review (4734866; status a02e379). Earlier broad recovery is superseded. Common-ui build/tsc and all 41 tests across 8 suites pass. Original user common-ui checkout untouched.

NGFW retained MUI7 overrides: updated common-ui explicitly supports MUI7/9 inputs, whereas NGFW schema fields still use MUI7 renderInput contracts. NGFW theme tests 5/5 and schema-form tests 29/29 pass. Web lint passes. Production build passed at 599.7kB gzip (600kB budget), with no dev routes. Settings popover is loaded on demand. Canvas CSS accepts solid colors as well as gradients.

Interface/setup integration tests still under observation; interface worker profile shows jsdom nwsapi CSS selector parsing/GC, not evidence of a render loop. Full hosted quick CI has not been run; not merge-ready. Browser verification blocked by browser tool URL policy; no workaround attempted. Preview restarted from repaired worktree at http://127.0.0.1:5174/ in sample-data mode, not connected to a device.
