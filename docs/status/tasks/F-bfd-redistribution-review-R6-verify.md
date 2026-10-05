# BFD — fresh R6 verify

Fresh independent R6/T4 verification, 2026-10-05. Reviewer owns /dev/shm/r6-verify and detached /dev/shm/r6-verify-ha snapshots, branch codex/review-r6-ui-verify-20261005; product sources unchanged. Historic BLOCK reports on f1a05f2a8a82186dc66fc5f4c5a270375db2ebbf remain unchanged. Dependencies/caches/logs/browser profiles and loopback Vite processes are private reviewer RAM resources. No shared DB/service/VPP/package/config writes.

Frozen local 2a5abf9e527ebdecf67c82c0f45ba54260bfcc9f; published a0dd57d1fd183d43a7dd26850f76bc7e9780bfc1. MAJOR R6-BFD-1 CLOSED: all known observed states translated in en/fa, unrecognized states use localized unknown without inventing Up. MINOR VRF context and loading/empty wording CLOSED. Duplicate source/target edges across multiple VRFs remain a producer-dependent historical limitation, previously explicitly nonblocking because actual producer emits one config per target. Production calls remain real API endpoints; no frontend mocks ship. MUI/logical CSS, schema-driven configuration permissions and navigation retained.

Independent actual tests: relevant App/BfdPage/BfdRedistributionPage suites **18/18 PASS**, 3 files, 133.08s. Old fifteen assertions retained with literal Down expectation localized. Reviewer closure probes **2/2 PASS twice**, 16.15s and 16.31s (actual component/QueryClient, labeled frontend fixture). Initial probe lacked docs-relative dependency resolution; private dependency symlink fixed fixture, no source changed.

Actual trusted headless Chrome frontend fixture: **8 screenshots PASS** across both pages en/fa, light/dark. html direction en=ltr/fa=rtl, localized observed Down, VRF blue/BLUE_FILTER and no browser uncaught exceptions asserted. See R6-verify-evidence/browser-bfd/browser-results.json and screenshots. Chrome intercepts HTTP requests with explicitly synthetic observations/authentication; these prove frontend rendering, not live backend, native packets or appliance acceptance. Initial browser /tmp inode failure fixed with owned RAM TMPDIR. No actual appliance stack screenshot claimed.

**Verdict: APPROVE**, zero open BLOCKER/MAJOR.
