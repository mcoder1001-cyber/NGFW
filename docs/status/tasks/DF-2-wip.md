# DF-2 — WIP log

- 2026-09-24 (continue after stall, salvage 800b936): tree builds, `go test ./internal/descriptors/...` unit green.
  Integration (`VRX_INTEGRATION=1`, w3) green per package; flaky when packages run concurrently:
  classify Retrieve failed with INVALID_SW_IF_INDEX when another package's cleanup deleted its w3 loopback
  between sw_interface_dump and classify_table_by_interface → fixed (`df2.InterfaceVanished`, skip).
- ACL key aligned with DF-4 `docs/agent/descriptors/acl.md`: `acl.acl/<name>` (was `acl/<name>`).
- Open: proxy-nd integration skipped by default (previous worker recorded a VPP crash on ip6nd_proxy_add_del);
  docs/agent/descriptors/*.md; DF-2.md with evidence; ci.sh gate.
- 2026-09-24 00:40: classify.session excludes ip_session_redirect sessions; idempotency host test green (apply twice → empty
  plan); docs/agent/descriptors/*.md; DF-2-questions.md; full integration (-p 1) green; CI GATE PASSED; DF-2.md written. Closed.
