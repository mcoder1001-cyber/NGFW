# Web startup WIP

Branch: codex/ngfw-web-startup
Worktree: /home/mohammad/ngfw-worktrees/web-startup
Owned files: NGFW web migration integration, packages/ui-kit theme, local shared-package staging tool and dependency configuration.
Previous local checkpoint: f538df8bb.
Remote checkpoint: c49d69a8a2935deea788b32398c96ad83a47fdc7 on codex/ngfw-web-startup-checkpoint (published via GitHub connector). New changes are not yet published.

Completed: integrated shared theme, restored interface aliases and raw action identifiers, resolved duplicate merge imports, restored optional WAN setup behavior, narrowed i18n metadata lint exemptions, lazy-loaded Login to reduce entry size. Independent review found spacing must remain a function with NGFW's 8px unit; corrected.
Actual checks before user update: web typecheck passed; web lint passed including logical CSS; browser dashboard rendered at port 5174 with sample data. Full web tests/build not complete; do not claim full CI green.

2026-10-19 user updated /home/mohammad/common-ui. Latest source supersedes earlier recovery; common-ui agent now reassesses that baseline in isolation. Original project source remains untouched by this task. Prior preview processes are no longer listening.
Remaining: build updated shared package, refresh isolated NGFW dependency, rerun typecheck/theme and web tests/production bundle checks, restart preview, browser verification, publish checkpoint. No merge authorized by passing checks yet.
Next command: inspect common-ui agent's new build result, then stage its validated package into .yalc and pnpm install using isolated store.
