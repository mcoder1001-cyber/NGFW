# Native certificate restart replay

Branch: codex/closeout-ikev2-cert-replay; base11526b81. Publication local only, parent owns publishing.
Owned scheduler descriptor/reconciler/new replay tests, vpn global forwarding/new test,
native ikev2 singleton/new replay tests, this status. No shared VPP/reference edits.

Design reviewed by traffic before implementation: optional in-place write-only Create
only when the current plan has ErrRetrieveUnsupported and exact actual KV is absent.
Real Create/setter and normal journal/error/uncertainty paths remain. No fake Retrieve.
Global owner forwards opt-in; Require does not. Native immutable key-HMAC and peer/key-HMAC
paths, snapshot verification, foreign RSA ownership and rotation retirement remain.
Contract commits before LocalKey consumer. Defaults/cached replays/other descriptors unchanged.

Actual pre-fix default-stock certificate proof failed18.81s on agent restart after real
certificate AUTHENTICATED, ICMP and 1MiB TCP passed. Evidence11526b81 retained.
Current implementation/checks pending. Next focused scheduler/vpn tests under tools/heavy.sh.

Completed contract3e172a4f followed consumer e51b7797; independent contract review87a8350e
and independent exact consumer review approved. Native consumer changes only opt-in.
Existing setter/snapshot/foreignguard/generation retirement/rollback compensation unchanged.

Actual validation on frozen e51b7797:
- scheduler/vpn/native descriptor race PASS (4.687 cached /1.178/1.265s), including
  immutable tampering/symlinks, foreign profiles, rotation retirement and uncertainty.
- scoped golangci-lint zero issues; production agent build PASS.
- default stock peer actual full certificate packet proof PASS29.87s/no skips:
  bidirectional ICMP/exact1MiB TCP/native counters, foreign SPI refusals, active Apply,
  actual production restart retained identical IKE/CHILD SPIs and protected traffic,
  rekey, route withdrawal/recovery, deletion fail-closed, ESP with no plaintext IPIP.
- unchanged production certificate lifecycle/rotation/revert/restart/removal PASS4.47s,
  package race5.846s, no skips.
- exact binary/plugin/source receipt and raw outputs in sibling evidence directory.

This is bounded RSA_SIG/SHA1 native initiator proof. DIGITAL_SIGNATURE(14)/SHA256
is unsupported, not accepted. Original default modern failure11526b81 predecessor and
unfixed-restart failures remain durable; corrected result does not relabel those.
No dynamic peer certificate chain discovery (native leaf preloaded on private peer),
no peer crash/responder/IPv6/FIPS/arbitrary certificate interoperability claim.
Full mandatory quick CI/integration/publication remain manager-owned before merge.
Shared VPP PID1014/NRestarts0/startup unchanged, slot8 cleaned.

Additional unchanged actual PSK responder regression on frozen e51b7797/agentbbcea
and patched plugin2c2079: original TestIKEv2NativePackets race PASS26.15s/package27.511s,
no skips. Exact1MiB TCP, counters, role-sensitive action checks, restart identical IKE/
CHILD SPI retention, route recovery/delete failclosed and ESP/no plaintext IPIP retained.
This covers new plugin responder compatibility without changing original assertions.
Final bounded independent certificate evidence review d8d8e4215a4af1850b1fc03594211264aef97220.
