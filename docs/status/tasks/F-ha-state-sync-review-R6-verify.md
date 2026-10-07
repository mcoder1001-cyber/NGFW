# HA — fresh R6 verify

Fresh independent R6/T4 verification, 2026-10-05. Reviewer owns /dev/shm/r6-verify and detached /dev/shm/r6-verify-ha snapshots, branch codex/review-r6-ui-verify-20261005; product sources unchanged. Historic BLOCK reports on f1a05f2a8a82186dc66fc5f4c5a270375db2ebbf remain unchanged. Dependencies/caches/logs/browser profiles and loopback Vite processes are private reviewer RAM resources. No shared DB/service/VPP/package/config writes.

Frozen local 075c062b7945fcb504e5a010a382f294bda8128b; published d4ecbddab46ff074ae968c737c1ac51a1b16015c. MAJOR R6-HA-1 CLOSED: failed observation query suppresses cached active endpoints and disables Resync; state fetching also disables action, agent observationError suppresses activity/actions. Administrator and actionsAllowed remain necessary; readonly/operator cannot Resync. Success action invalidates observation query, backend remains authority. Locales, unsupported capability notices and native counter unavailable wording truthful.

Independent actual HA/StateSyncPanel suites **12/12 PASS**, 2 files, 16.65s. Reviewer success→failed refresh regression **1/1 PASS twice**, 6.64s/6.08s, asserting no cached active text and disabled Resync. Initial docs-relative dependency resolution corrected using private dependency symlink. No assertion weakened.

Actual trusted Chrome frontend HTTP-fixture matrix **4/4 views PASS**, en/fa, light/dark, html ltr/rtl. Selected actual Cluster tab through navigation; after successful active observation, fixture GET returns503; after unchanged production 30s polling interval, active text disappears and Resync is disabled. No uncaught exceptions. See R6-verify-evidence/browser-ha/browser-results.json and screenshots. Browser intercepts HTTP with labeled synthetic auth/state, not actual native/appliance observations. Initial script selected default VRRP tab rather than Cluster, corrected fixture navigation; stale owned DevTools port file removed before relaunch. Neither was a product failure. Actual second-node/native-session continuity and appliance browser stack NOT RUN; no such claim.

**Verdict: APPROVE**, zero open BLOCKER/MAJOR.
