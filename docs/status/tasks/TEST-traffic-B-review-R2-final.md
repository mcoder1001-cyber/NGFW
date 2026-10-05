# TEST-traffic-B final independent R2 delta review

Reviewer branch/worktree: `codex/traffic-final-security-review-20261005`, `/root/ngfw-wt/traffic-final-security-review-20261005`. Owned files only this report, reviewer envelope/WIP and independent probes. Initial coherent source3e940102 against previous e37 R2 approval; final coherent DHCP repair pin reviewed: eb1a634da6858e3c6599e9586044b5030309f39b. No product edits, fullquick duplication or live-host allocation.

## Immutable3e changes

No new security finding in this bounded part. The Go bridge restricts phase names to explicit values; evidence runtime paths reconstruct a numeric private wrapper PID below repository scratch and must match the received path exactly. Go os.Root operations confine diagnostic creation; runtime/evidence directories must be owned0700 and log creation is exclusive0600. gRPC owner Retrieve remains an actual bounded10s request before starting the private REST bridge; NewClient/Connect does not bypass owner checks. Existing Python SO_PEERCRED, recorded PID, private mount/net namespace and owned executable/parent guards remain. Separate responder/initiator runtime stack identities avoid resource collisions and permit their independent proofs; only literal r/i variants are accepted.

Actual reviewer commands/results in frozen3e worktree:

```text
tools/heavy.sh env GOMAXPROCS=2 GOFLAGS=-p=2 go run .scratch/r2-evidence-root.go
owned0700 runtime/evidence and0600 exclusive log: PASS
outside-root directory symlink refusal: PASS
symlink log refusal without outside write: PASS
wrong directory mode refusal: PASS
foreign directory UID refusal: PASS
python3 -m unittest discover -s test/topology/traffic-b -p test_scenario.py -v
Ran 16 tests in 1.018s
OK
```

The retained Go probe exercises the same os.Root operations and directory checks independently in owned temporary directories, with actual symlinks/file modes/UID changes. It does not invoke the full live Go REST fixture; that remains assigned to T3. All temporary files/commands finished.

## Historical checkpoint remaining work (closed below)

Await one final coherent DHCP repair pin. The old DHCP unsupported-warning false acceptance is a real defect and the prior canceled fullquick exit143 is NOTPASS. Final review must establish failclosed applied/notApplied/concrete-revision and owner/hash proof acceptance; only byte-equal observed warnings for explicitly disabled unchanged unrelated defaults may be exceptions, never changed DHCP/interfaces. Verify evidence carries only owner/hash/revision/result metadata and no raw candidate or secrets. No final R2 approval or fulltask Done is claimed here yet.

Initial immutable3e secret scan completed independently:

```text
gitleaks dir --config .github/gitleaks.toml --redact .
scanned ~73784689 bytes (73.78 MB) in 9.2s
no leaks found
```

## Final coherent repair closure — eb1a634da

Reviewed all coherent DHCP repair changes against3e, source `eb1a634da6858e3c6599e9586044b5030309f39b` (author tree626005f1375a917fdfb9c248f98ad16215fbd964). No mutable author file was tested. The acceptance guard rejects HTTP errors, non-applied commits, non-empty/malformed notApplied, unsupported object results, and absent/nonpositive/nonintegral revisions. Candidate lock owner is observed before and after reading the actual candidate; the committed running document must have the same canonical SHA256. Rollback similarly checks the actual baseline hash.

The only unrelated warning exceptions are six fixed, full, current schema-default objects: external AAA unused/local-only with MFA none; custom TLS material absent (API TLS itself is NOT disabled); flowprobe interfaces empty; backup/NATIPFIX/NTP explicitly enabledfalse with their exact complete default values. Before and candidate values must be equal. Every post-baseline warning must equal a previously observed baseline warning object. Missing/empty nodes, enabledtrue, unknown/custom controls, changed warnings/values and changed DHCP/interface warnings are refused. Static expected defaults are not learned from the current candidate. Future schema-default changes therefore fail closed until deliberately reviewed. This addresses the earlier interim equality-only weakness and does not widen production API/agent validation or disable warning emission.

New DHCP proof output contains only transaction identity, owner, canonical digest, applied revision/status, notApplied/warning/baseline-warning metadata. It does not serialize a raw candidate, password, bearer token or private key. Proofs live in the existing owned0700 campaign/evidence directories and0600 phase diagnostics; the aggregate JSON is contained by that owned0700 directory. The unchanged private login and secrets API channels stay separate.

Actual independent final-source checks:

```text
tools/heavy.sh env GOMAXPROCS=2 GOFLAGS=-p=2 go -C test/topology/kea-dhcp-relay test -count=1 -run '^(TestCommitAcceptance|TestConfigDigestCanonical|TestBaselineWarningGuard|TestInactiveBaselineRequiresRecognizedExplicitDefault)$' -v ./...
--- PASS: TestCommitAcceptance (0.01s)
fully-applied / changed-DHCP-unsupported / partial-apply / unsupported-result / missing-revision / non-applied subcases all PASS
--- PASS: TestConfigDigestCanonical (0.00s)
--- PASS: TestBaselineWarningGuard (0.00s)
--- PASS: TestInactiveBaselineRequiresRecognizedExplicitDefault (0.00s)
all six pointer subcases PASS (recognized default accepted; nil/empty/active/unknown controls refused)
PASS
ok ngfw/test/topology/kea-dhcp-relay 0.033s
python3 -m unittest discover -s test/topology/traffic-b -p test_scenario.py -v
Ran 17 tests in 0.772s
OK
python3 .scratch/r2-closure-replay.py
protected actual SO_PEERCRED positive: PASS (fixture executable observation mocked)
foreign observed owner refusal: PASS
foreign/shared PID refusal: PASS
foreign mount namespace refusal: PASS
unprotected socket parent refusal: PASS
owned relay0700/socket0600, fixed endpoint, byte transfer and cleanup: PASS
gitleaks dir --config .github/gitleaks.toml --redact .
scanned ~73821429 bytes (73.82 MB) in 8.8s
no leaks found
```

The replay source is retained as `TEST-traffic-B-security-owner-probe.py`, executable from repository root. Actual UNIX peer credentials and namespace reads are used; the positive .test executable observation is explicitly mocked. Relay forwarding uses a socketpair backend and asserts the exact fixed PostgreSQL target; no database traffic is sent. Every owned command, temporary socket, fixture and thread finished. No live27/28 allocation, shared VPP/service mutation, production code edit, or fullquick duplication was performed.

Final verdict: **APPROVE**, R2 only, for eb1a634da; zero open R2 BLOCKER/MAJOR/MINOR. Actual all-seven packet campaign and complete unchanged quick CI on this new final source remain required from the assigned testers; prior source success and canceled gates are not substitutes. Production WireGuard secret-channel limitation remains explicitly documented as before.

## Fresh numeric LockOut repair closure — 8e1c41d59

Exact fresh source `8e1c41d59ef177e81dfec3ed095c317c20b36dd5`, published `d6a2ef2df0fc3b40c06bb227ed68fa9b72f51092`, root product tree `da4c9d727dde9981c923a49864974d659051d995`. Against prior eb review, four changed test-source files correct the actual numeric LockOut.ownerId contract. Other incoming changes are documentation/task-board integration of already reviewed main. Prior eb approval is not blindly carried: this section reviews/tests the new numeric acceptance explicitly.

No new R2 finding. candidateOwner accepts only an HTTP200 locked=true positive integral safe JSON number (decoded float64); display strings, booleans, fraction, zero, negative, unsafe, null, missing owner, and unlocked state are refused. Both lock reads use this same validated type; a foreign changed numeric owner causes failure before POST commit. Python proof parsing requires an actual positive safe int and explicitly excludes bool/string/fraction. The candidate SHA256/running readback and exact inactive-default warning checks remain. Evidence contains the numeric account ID and metadata only; no credential/token/private key/raw candidate is introduced. No API/agent production, filesystem/privacy/namespace guard, privilege rule, dependency or secret channel changed by this repair.

Actual independent commands in the frozen own review worktree:

```text
tools/heavy.sh env GOMAXPROCS=2 GOFLAGS=-p=2 go -C test/topology/kea-dhcp-relay test -race -count=1 -run '^(TestCommitNumericOwner|TestCandidateOwnerWireContract|TestCommitAcceptance|TestBaselineWarningGuard|TestInactiveBaselineRequiresRecognizedExplicitDefault)$' -v ./...
--- PASS: TestCommitAcceptance (0.04s)
--- PASS: TestBaselineWarningGuard (0.00s)
--- PASS: TestInactiveBaselineRequiresRecognizedExplicitDefault (0.01s)
--- PASS: TestCommitNumericOwner (0.02s)
--- PASS: TestCandidateOwnerWireContract (0.04s)
(all10 numeric/display-name/boolean/fractional/zero/negative/unsafe/missing/null/unlocked cases PASS)
PASS
ok ngfw/test/topology/kea-dhcp-relay 1.194s
python3 -m unittest discover -s test/topology/traffic-b -p test_scenario.py -v
Ran 17 tests in 0.986s
OK
gitleaks dir --config .github/gitleaks.toml --redact .
scanned ~73876707 bytes (73.88 MB) in 9.66s
no leaks found
```

Independent whole api.commit foreign-owner probe uses an overlay only (product source untouched). Its actual HTTP fixture returns owner1 for the first lock and owner2 for the second. The subprocess must fail with candidate ownership changed and must not send POST.

```text
tools/heavy.sh env GOMAXPROCS=2 GOFLAGS=-p=2 go -C test/topology/kea-dhcp-relay test -overlay=/root/ngfw-wt/traffic-final-security-review-20261005/.scratch/r2-owner-overlay.json -count=1 -run '^TestR2ChangedOwnerRefusedBeforeMutation$' -v ./...
=== RUN TestR2ChangedOwnerRefusedBeforeMutation
--- PASS: TestR2ChangedOwnerRefusedBeforeMutation (0.03s)
PASS
ok ngfw/test/topology/kea-dhcp-relay 0.046s
```

Retained probe `TEST-traffic-B-security-numeric-owner-probe_test.go` maps to virtual package file `test/topology/kea-dhcp-relay/r2_numeric_owner_test.go`. Existing whole-helper positive test proves all six actual HTTP lifecycle steps, applied revision42 and exact committed candidate hash readback. No live host/slot27 campaign, production edits or fullquick duplication occurred. All owned commands/processes finished.

Current source-specific verdict: **APPROVE**, R2 only, for8e1c41d59; zero open R2 BLOCKER/MAJOR/MINOR. Actual DHCP/all-seven and complete mandatoryquick still belong to assigned testers. Later expanded committed evidence requires final-tree secret scanning and product-diff confirmation; no automatic stale approval is asserted.
