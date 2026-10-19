# Route Ethernet9 ↔ Ethernet11 on appliance 211

- User request: configure `https://172.30.110.211/interfaces` for bidirectional routing between interfaces 9 and 11.
- Owner: root operational agent; branch `codex/route-ethernet9-11-211-20261019`; worktree `/root/.codex/worktrees/b538/NGFW`.
- Owned files: this envelope and `route-ethernet9-11-211-20261019-wip.md` only. No product code changes planned.
- Independent reviewer: `routing_review`, branch `codex/review-route9-11-211-20261019`, worktree `/root/ngfw-wt/review-route9-11-211-20261019`; read-only device/source review.
- Scope: inspect exact port identities and existing configuration, obtain missing addressing/access, prepare a narrow transactional candidate change, validate, apply with confirmed commit, verify live forwarding and persistence, report actual evidence.
- Preserve management interface enp4s0, unrelated ports, credentials, existing candidate edits, VPP startup configuration and running services. No VPP restart, package deployment, auth bypass or database configuration writes.
- User steering: first enable Layer 2 for initial testing, then move to Layer 3. User supplied a normal web administrator login; credentials must not be recorded here. Authentication succeeded through HTTPS.
- Initial test configuration: reciprocal `routing.l2.xconnects` for the two physical ports. Layer 3 addressing remains unspecified and must not be invented.
- A pre-existing agent namespace-readback bug blocks validation. Reuse and review the published PR249 correction on the installed main source before deployment; do not bypass base-policy validation or change privileges.
