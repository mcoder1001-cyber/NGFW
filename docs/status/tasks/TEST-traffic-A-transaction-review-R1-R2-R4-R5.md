# TEST-traffic-A fixture transaction consistency — R1 / R2 / R4 / R5

2026-10-02. **APPROVE** source-only phase frozen
`0cbfa17cad3063cf914efd7f587395f22abc33f0`. Own isolated worktree
NGFW-traffic-transaction-source-review, branch task/traffic-transaction-source-review.
Only this report authored. No product/test or arbitration authorship.
No BLOCKER/MAJOR; optional R5 clarification below does not authorize live use.

R1: explicit expected candidate digest/revision bind the fixture; exact task/slot/
prefix/boot/run/lease fields, finite bounded expiry before/after observation and
exact two-sided typed identity compare twice. Snapshot signature+digest rechecks
both lease/binding to detect change/revocation, distinct namespace identities and
exact field sets prevent ambiguous consistency. Return is only
FIXTURE_TRANSACTION_CONSISTENT with all three proof flags false. It is not applied
config, actual lock possession, rollback, atomic trust or authoritative ownership.

R2/R4: opened NOFOLLOW directory descriptor must be owned0700; same-dirfd opened
NOFOLLOW/NONBLOCK/CLOEXEC owned0600 single-link regular JSON files1..8192bytes;
same bytes parsed after size/inode/mtime/ctime stability, duplicate and nonfinite
JSON rejected. Descriptor finally closes on failure. Live refusal occurs before
filesystem/observer work. Private fixtures use current uid, not invented root
authority. tools/lab source really reuses namespace/veth/VPP names and emits no
capture-binding/traffic lease issuer; matching names/metadata do not prove owner.
No real observer/backend, lock, run.py/producer activation, API/YANG/VPP binding,
capability or global shared-host privilege added; unchanged entry source confirmed.
README/envelope explicitly preserve this source gap, not lab-only deferral.

R5: JSON byte8192/read4096 and four typed observations bound ordinary fixture work;
positive integer identity excludes bool/zero, canonical lowerhex SHA/run/UUID/MAC,
finite caller clock/expiry and <=7200 lease duration checked. No external command,
process group or capture operation introduced. MINOR clarification: injected
Python observer/clock callbacks are trusted fixture code; this helper has no
execution deadline around callbacks. Envelope phrase 'bounded read-only ...
observations' describes required future adapter behavior, not a measured timeout
guarantee in current helper. Future real backend must independently bound calls
before any live enablement. This is compatible with mandatory early live refusal.

Actual independent execution:
`python3 test/topology/traffic-a/check.py`:47 tests in5.005s PASS;
failures=0 errors=0 skipped=0 expectedFailures=0 unexpectedSuccesses=0.
Seven new cases meaningfully exercise actual private files/FIFO/hardlink/symlink,
wrong mode/uid/size, exact candidate and lease mismatch before observer, duplicate
JSON, mutation before any parse, namespace alias/change, revocation/expiry during
callbacks and patched live-open/observer assertion proving no live operations.
Four additional reviewer controls: infinite clock, bool ifindex, zero namespace
handle and malformed MAC all refused; assertions PASS. No mirrored product tests
written. Historical producer BLOCK/independent closure and A3 split preserved.

No real SSH/network/ip/netns/VPP/nft/tcpdump/lease issuer/host locks or mutations
executed. This approval is not producer/transaction whole DONE, live provenance
or whole TEST-A completion. New CI47 companion and remaining panels, final actual
main composition plus unchanged complete hosted quick/exact47 remain required.
