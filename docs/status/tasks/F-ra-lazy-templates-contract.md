# Lazy strongSwan templates contract

Branch codex/ra-lazy-templates-20261006. Exact base local605bb2beb729ca7960ffe2d48dcf0a45dfd96cfe/remote4fb5bf4833a3a138015e6b322519d17379ac7436/tree3d4099932ef17c598a63e8ebe62a5dc84f618e26.
Owned product files: apps/agent/internal/renderers/strongswan/renderer.go global-template/import/three-render-call hunks and new lazy_templates_test.go only. Owned docs: this contract, F-ra-lazy-templates-wip.md and F-ra-lazy-templates-report.md.

Replace eager template parse with zero-initialized private loader (sync.Once plus nil function). Its get(parser) initializes sync.OnceValue(parser) inside outer Do, then calls the cached loader outside Do. Production accessor always supplies unchanged trusted parseTemplates: same NewTemplate/FuncMap/embedded FS/Must/templates. Exactly three renderer calls use shared accessor. No eager call, per-render parse, public injection/testhooks/global resets, guards/deadlines/buildflags/units changes. Parse panic moves from package startup to first rendering and is replayed with same value on every later call.

P11 design read first approved the zero-loader pattern and isolated instance tests. Actual concurrency tests must prove zero initialization/no premature parse, exactly one parse, identical pointer, identical panic object replay and unchanged render goldens. All strongSwan race tests and contextual lint required; independent source review before integration or own fresh helper invalid-args measurement. No operational Ready claim.
