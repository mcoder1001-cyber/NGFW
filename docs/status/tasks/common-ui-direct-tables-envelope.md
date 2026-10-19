# Direct native table migration

User explicitly requested direct ReactGeereedMaterial imports with native columns/items instead of handwritten Table/Head/Row/Cell markup. Reuse codex/common-ui-migration-20261008 in /Users/mohammad/.codex/worktrees/1263/NGFW. Keep RHF/SchemaForm. No commits, pushes or rebases. Preserve preexisting Common UI integration changes.

Own the 58 handwritten-table page files, focused regression tests and these notes. Common UI owns additive header cell props and controlled detail visibility in src/components/pg-grid/{ReactGeereeedMaterial.typings.d.ts,GeereedHeader.tsx,GeereedRow.tsx}, plus tests/docs and local distribution refresh. ServerDataGrid consumers are outside this handwritten-table conversion; they already render the original component through their server API adapter.

Preparation agents only edited isolated /private/tmp copies; root applies product edits to the selected branch. Backups: /private/tmp/ngfw-direct-tables-baseline and /private/tmp/ngfw-native-library-baseline. Tests are written/run after implementation, per user.
