# F-igp-followups task envelope

Date: 2026-10-04. Branch: codex/igp-followups-20261004. Isolated worktree: /root/ngfw-wt/igp-followups-20261004. Base: origin/main d5557440c.
Manager: current root worker. No laboratory slot requested; host-independent development.
Owned: packages/schema/src/domains/routing.ts and corresponding schema test/generated outputs; packages/proto/ngfw/v1/dataplane.proto and generated outputs; apps/agent/internal/renderers/frr/rip/**; ripng authentication rejection; docs/user/routing/isis-rip.md; own task status.
Historical scope is already implemented in main by OSPF completion, #151 IS-IS/RIPng and #155 HA. Actual missing source is RIPv2 authentication. Additive interface auth contract uses the existing none/md5/keyId/keyRef object; no production secret-channel change. Commit contract before consumer. Manager integrates sequentially after independent review and unchanged complete quick gate.

Actual expanded ownership: apps/api/src/secrets/secret-delivery.service{,.test}.ts (routing auth selection only, coordinated with wan_pppoe's separate interface selection), apps/web/src/domains/routing/isis-rip/IsisRipPage.tsx and ospf/locale.ts (en/fa guidance), apps/agent/internal/contracttest/drift_test.go (explicit shared-message RIPng exclusion).
