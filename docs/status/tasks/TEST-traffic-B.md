# TEST-traffic-B — REST traffic acceptance

Status: running; corrected-source local primary acceptance and complete independent T1 PASS, independent final R7 APPROVE; hosted/main CI pending. This report does not mark Done before those gates finish.

Every primary configuration traverses an owned authenticated REST candidate/commit API into the real VPP-backed agent. Candidate ownership, canonical digest/running readback, concrete revision, applied status, empty notApplied and unsupported-field guards precede actual packet assertions. Native PSK responder and initiator have separate REST proofs. Current native VPP route-based scope follows [the owner decision](../../decisions/DEC-ipsec-route-based.md); obsolete NGFW-side strongSwan/kernel-vpp/P11 build requirements are superseded. Stock strongSwan remains the interoperability peer.

## Executed corrected-source acceptance

Product/test source `43cfd1c9217163745246fb8816399756e70ad71b`. ROOT froze `d3c33570fd7c1af37361c0c9fb63e577c988cd45`, tree `1da6b378e4bfc49f4646513c5165fd31bb9e061a`; differences from43c were docs only. Session33969 exited0, 2026-10-05 20:35:55–20:44:35UTC:

```sh
NGFW_INTEGRATION=1 GOMAXPROCS=2 GOFLAGS=-p=2 python3 test/topology/traffic-b/run.py --slot 27 --output .scratch/traffic-root-all7-readiness
```

Actual output:

```text
START ipsec
END ipsec passed
START ipsec-cert
END ipsec-cert passed
START wireguard
END wireguard passed
START tunnels
END tunnels passed
START bgp
END bgp passed
START ospf
END ospf passed
START dhcp-relay
END dhcp-relay passed
```

[ROOT aggregate, exact source and proof hashes](TEST-traffic-B-root-readiness-evidence/composed-summary.json). Independent T3 repeated the unchanged complete command in slot28 on frozen `ccfde07d2c1c50cfaedbd83b63ceea6fe89de9a8`, identical43c product/test source, session18850 exited0, 20:45:43–20:54:38UTC. [Independent report with actual command/output](TEST-traffic-B-test-T3-readiness.md) and [curated API/readback/packet evidence](TEST-traffic-B-independent-T3-readiness-evidence/composed-summary.json).

| Primary phase | ROOT | Independent T3 | Executed observations |
|---|---|---|---|
| Native PSK responder and initiator | PASS | PASS | Two applied REST proofs; ICMP/TCP, ESP/no plaintext, rekey, withdrawal/recovery, restart, owned rollback |
| Native certificate IPsec | PASS | PASS | Production agent, stock RFC7427 peer, applied REST, packets and rollback |
| WireGuard | PASS | PASS | Applied REST, positive kernel handshake, inner ICMP/scoped UDP, restart and rollback; authorized tagged secret fixture |
| GRE/VXLAN | PASS | PASS | Persisted candidate digest/revision, kernel peers, ICMP/encapsulation, empty state after rollback |
| BGP | PASS | PASS | Actual learned FIB routes, exact TCP payload/ICMP, policy/link and restart recovery, applied rollback |
| OSPF | PASS | PASS | Learned FIB/ICMP/TCP, withdrawal/reannounce, restart recovery, applied rollback |
| DHCP relay | PASS | PASS | Numeric candidate owner1, two applied commits/revisions1/2, exact candidate hashes, DISCOVER giaddr/lease, restart, one applied rollback revision3 and empty relay state |

DHCP ROOT29.154s (restart4.28s, single rollback0.81s, cleanup0.79s); independent30.862s (restart4.71s, single rollback0.85s, cleanup0.91s). The fresh fixture waits for authenticated GET `/api/v1/state/dhcp/relays` to execute real agent Retrieve and report the exact relay applied, inside the original shared30-second recovery deadline. It keeps one strict rollback POST, with no retry, fixed sleep, deadline increase or weakened assertion. Independent readiness was observed at+1.50s. Cached health/listening or an empty/mismatched relay cannot satisfy it.

After both complete commands, owned namespaces, API/Valkey/UI listeners, processes and PostgreSQL roles/databases were absent. Global acceptance lock was released. Shared VPP remained MainPID1014/NRestarts0. Raw private captures, keys, tokens and daemon logs remain ignored scratch; only reviewed public metadata/text is committed.

## Guard and scope limits

D-237 permits only six exact unchanged schema-default objects with exactly observed unsupported warning objects, including message. AAA is local-only with no external providers; TLS has only minVersion1.2 and no custom certificate/key refs. This does not disable AAA or API TLS. Unused flowprobe has no monitored interfaces; backup, NAT IPFIX and NTP have exact disabled default objects. Missing, empty, custom, newly enabled, new-shape or changed DHCP/interface/VRF fields refuse acceptance; public warnings remain verbatim. Other informational warnings such as LLDP write-only may remain. Strict positive safe-integer numeric ownership is observed before candidate mutation and commit; string/fraction/foreign/missing owners and partial/notApplied/unsupported changes are rejected.

WireGuard uses the task-approved `ngfwtestsecrets` fixture and locally signed short-lived licence verified by production licensing guards. Production WireGuard secret transport is not established by this test. Agent lastHandshake may remain null; actual kernel handshake, ping and UDP capture are retained. Native warning prerequisite [PR192](https://github.com/mcoder1001-cyber/NGFW/pull/192) is merged and exact hosted/main quick passed.

Both aggregates report `primary_passed:true`, `whole_wave_passed:false`. Eight optional smokes are NOT RUN, with individual reasons in each aggregate: det44/CNAT private globals window; DS-Lite deletion prohibited D211; IS-IS private OSI window; BFD dedicated evidence export; unbound DNS, chrony source and syslog receiver exports. These are not primary PASS claims.

## Independent review, CI and preserved failures

[R1 fresh-source bounded review](TEST-traffic-B-readiness-r1-preflight.md) and [R2/R4/R8 fresh-source review](TEST-traffic-B-review-R2-R4-R8-readiness.md) approve the correction. [Final independent R1 APPROVE](TEST-traffic-B-readiness-r1-review-R1.md) and [complete independent T1 PASS](TEST-traffic-B-readiness-r1-test-T1.md) are recorded on exact43c: unchanged quick session36602 exited0, wall28m57s;35/35 workspace tasks,139 agent race-tested packages,27 Go modules, CLI and guards, five shellcheck files, and149 actually executed startup checks (59+90, zero failures). [Unabridged actual quick output](TEST-traffic-B-readiness-r1-full-quick-output.txt). Tested ownHEAD1b044c73d6fa7ee221f59ac84dbce7deaab90f6e differs from43c only review documents; fresh actualmain3c99 likewise adds only four status documents above testedbase4788. [Independent final R7 APPROVE](TEST-traffic-B-final-review-R7.md) verifies fresh T1/T3, source attribution,29 local links, full212-row preservation and scope/history. Exact final hosted quick and actual-main CI remain pending. Historical8e quick PASS does not substitute for this new-source gate.

[Historical canonical report](TEST-traffic-B-pre-readiness-history.md), [ROOT attempts](TEST-traffic-B-root-attempt-evidence/reproduced-composed-fail.json), and previous independent reports preserve original unsupported guard, numeric owner, dependency/build and environment failures. Two8e composed runs reproduced DHCP post-restart rollback503. They were reproducible BLOCK, not FLAKY; fresh43c readiness correction and two whole-run positives close that defect. Original certificate FLAKY remains recorded with owner/due in tech-debt; no new DHCP debt waiver was created. [Arbitration follow-through](TEST-traffic-B-arbitration-follow-through.md) records three fresh-author handoffs without changing the ruling.
