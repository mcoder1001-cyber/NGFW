# Common UI sample WIP

Branch: `codex/common-ui-sample-20261007`.
Base local SHA: `e74c33eb`.
Observed remote HEAD: `62325fd4d4e38fd330aa2df9af85a9a06d2d4ef9` (git ls-remote).
Remote checkpoint: not yet published.
Checkpoint attempt: git staging escalation was rejected by the user. No commit or
push was attempted afterwards. All sample files remain untracked in the worktree.
Owned files: `prototypes/common-ui/**`, `docs/status/tasks/common-ui-sample*`.

Completed: reference frontend/docs/template inspection; real Common UI standalone
Interfaces preview; schema-preserving form adapter; local stage/review/apply/discard;
independent form review. Invalid drafts are dirty even when schema validation fails.

Actual checks: dependency preparation passed (Common UI 2.0.169); dependencies resolved
and installed. The first install exited 1 because Kendo's licensing script was ignored;
the preview now explicitly disallows that script. TypeScript passed after MUI 9 Stack
props were moved into sx. Vite bundle built (21.18s, warnings for Common UI barrel's
eval and large chunks). Vitest: 1 file, 3 tests passed (661ms). Browser showed Interfaces
and the Pg editor; MTU 67 was rejected by NGFW schema and Stage changes disabled.
The draft discard warning appeared. Native browser confirmation stalled the controls;
it has been replaced by an in-page MUI dialog. Final browser interaction pending.
No product merge.

Final checks: TypeScript exited 0; Vite build exited 0 (17.06s); Vitest 3/3 passed
(598ms). Browser verified invalid MTU rejection, in-page discard confirmation,
Keep editing preserving draft, valid MTU staging, review showing old/new config,
Apply preview success with pending count returning to 0, and dark theme. Screenshot:
`/private/tmp/ngfw-common-ui-preview.png`. Independent review APPROVE for isolated
sample only. Main page shown in a deliverable browser tab at port 5188.

Owner approved the sample and requested all pages. Production migration continues
on `codex/common-ui-migration-20261008`; see common-ui-migration-wip.md for current
implementation/check results. This prototype is retained as the approved design
reference and uses explicit sample data only. Remote publication failed HTTP403.
The old prototype preview is separate from the real application preview on5189.
