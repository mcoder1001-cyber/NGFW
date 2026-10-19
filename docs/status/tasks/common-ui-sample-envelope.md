# Common UI sample task envelope

Owner request: redesign the whole NGFW UI to match `/Users/mohammad/webngf/frontend`,
using Common UI throughout. First deliver one sample for visual review. Common UI
changes are authorized if needed. Evaluate `usePgForm` for component compatibility.

Branch: `codex/common-ui-sample-20261007`.
Worktree: `/Users/mohammad/.codex/worktrees/1263/NGFW`.
Owned files: `prototypes/common-ui/**` and `docs/status/tasks/common-ui-sample*`.
Scope: isolated, working Interfaces design preview and safe schema-backed form adapter.
Production/API/data-model migration is deferred until sample feedback.
Independent reviewer: `/root/form_review`, read-only; owns no product files.

Acceptance: real Common UI components, reference frontend layout, working edit,
stage/review/apply/discard preview, schema adapter tests, build, browser visual and
interaction checks. Full hosted quick gate is mandatory before any merge.
