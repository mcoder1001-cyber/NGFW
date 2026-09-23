# Tech debt / backlog for idle workers

The manager pulls from here when nothing on the board is ready. Add items with a one-line why.

- [ ] golangci-lint binary install + config wired into `tools/ci.sh` (D-009)
- [ ] Remove libvirt/virbr0 from the host (installed by mistake; harmless) — needs product-owner ok? no: it is ours, purge it
- [ ] `packages/api-client`: switch `openapi-fetch` calls to typed helpers once P06 lands
- [ ] Generate the missing `prompts/features/F-*.md` files from FEATURE-TEMPLATE (one per board task) — do this early, it is pure text work
- [ ] `docs/user/` skeleton (MkDocs) so DOCS-GEN has a target
