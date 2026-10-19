# Original Common UI integration task envelope

Requested result: replace NGFW component imports with shared Common UI controls maintained in `/Users/mohammad/webngf/common-ui`; retain existing consumers and postpone the Common UI commit until the owner requests it.

Owned NGFW scope: apps/web component imports, packages/ui-kit component imports, dependency manifests/lock/config, removed packages/common-ui snapshot/import tool, tools/update-common-ui.sh, and this task's documentation. Branch codex/common-ui-migration-20261008; local checkpoints09d39fdd and fac6a4fd. No remote publication confirmed.

Original library scope: transferred native grid/shell/form/theme integration edits; new src/compat, scoped exports, autocomplete slots helper, additive package exports, one multi-entry build, original compatibility tests and documentation. Original branch issues/PGWUI-3339 at1a8e241 remains uncommitted. Preserve preexisting344mode changes, original version edits, and font/binary edits. Transfer inventory is documentation/ngfw-transferred-files.json in the original library.

Independent reviewer form_review approved source and packaged context compatibility after19focused tests passed. Source lives only in the original library. NGFW's ignored .yalc directory contains its built distribution, not a separately maintained implementation.
