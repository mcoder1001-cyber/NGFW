# TEST-traffic-A correlation — independent source review

Frozen `de8fedb1252beb694329ca86321cea424c3dad18` (product f0fe88ac), isolated `task/TEST-traffic-A-correlation-review`. Reviewer did not author product/tests. This agent previously issued the phase arbitration and now performs reviewer duties; it is ineligible to arbitrate a future dispute on this reviewed continuation. Scope R1/R2/R5/R7; R6 has no UI/locales changes and is not applicable. R4/R8 coverage remains the manager’s separate applicable dispatch responsibility.

**APPROVE the bounded inactive source continuation for these scopes.** No MAJOR/BLOCKER found for its stated contract. This is not a whole-task approval, live packet PASS, complete panel decision or hosted quick result.

Personally executed on the frozen isolated tree:

```
python3 -B test/topology/traffic-a/check.py
Ran 28 tests in 2.457s — OK
traffic-a source fixtures: tests=28 failures=0 errors=0 skipped=0 expectedFailures=0 unexpectedSuccesses=0

git diff --check
EXIT0, no output

Additional temporary in-memory adversarial checks using existing packet fixture helpers:
wrong TCP sequence, ACK, earlier output timestamp, flags; NaN window;
65 probes; duplicate probe identity; IPv4 MF fragmentation
Adversarial cases refused: 8/8
```

Tests create private temporary captures and only their own controlled Python processes. No real network/SSH/VPP/netns/nft/tcpdump, lease or global host operation occurred. Full unchanged hosted quick was NOT RUN by this reviewer and remains mandatory on the final integration tree. Author’s earlier local check is distinguished from that full gate.

R1: Both separate captures are required and same-inode aliases rejected. Typed per-probe identity plus exact flow, VLAN, TTL, payload hash, TCP ACK/flags or ICMP identifiers prevents merely counting output frames as a match. Missing/duplicate ingress cannot prove drop; any same probe identity on egress refuses drop, including transformed flow/payload. Forward rejects translation; source NAT preserves remote endpoint, reverse NAT preserves remote source, endpoint independence requires same internal endpoint and one stable translated mapping across distinct destinations. ECMP requires every declared next-hop MAC; PBR requires one. Both windows bracket probes plus positive quiet settle; output delay is bounded. Fragment/truncation and duplicate/replay identities fail closed. Drop cause and configured path semantics remain unproven, as README states.

R2/R5: Fixed argv/no shell and own-session cleanup retained. Output streaming has an explicit disk byte cap, bounded read chunks and monotonic timeout; noisy producer regression checks exact4096-byte private output and producer death, while descendant cleanup regression remains. Captures <=16MiB, <=2048 records, <=64 probes; worst matching cost is bounded by these limits. No dependencies, privileges, public contracts, global signals or external activation added. New source fixtures contain no secrets.

R7: Envelope and README explicitly separate `FIXTURE_CORRELATED` / `PACKET_EXPECTATIONS_MATCHED` from traffic PASS, keep live provenance/whole-chain/packet proof false, and leave capture producer, composed executor and transaction/lock lifecycle genuinely NOTIMPLEMENTED. Historical BLOCK and later foundation approval preserved. No unimplemented source is disguised as lab-only deferred acceptance. Board not edited here. User-facing R6 changes absent.

MINOR / future activation requirements (not permission to enable live execution): capture currently lstat-checks then reads a path, so future trusted capture producer must establish protected run-directory/file ownership and race-safe acquisition; labelled metadata and hashes are consistency checks, not independent provenance. Parser does not establish IP/TCP/ICMP checksum validity or causal config/counter linkage. These limitations are compatible with current false proof flags and refused live CLI, but must be resolved or explicitly fenced in the actual live executor review before any valid packet/drop acceptance claim. Matching bytes alone cannot attest forwarding correctness or explain an ACL versus uRPF drop.
