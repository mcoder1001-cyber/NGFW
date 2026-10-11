# PPPoE and RA completion work — acceptance remains open

Base main d44c68edccd16e8adc9ac2545c0b61f5154ed36e; 210/212 merged.

PPPoE primary-address repair: after the existing actual kernel forwarding, lease,
namespace, MTU, VLAN, cross-connect, invocation and NCP proofs, retain the learned
mirror before writes and validate the live owner/index plus exact native TAP ID,
namespace, host name, MTU and absent bridge before each operation. Only the
negotiated IPv4 /32 and deterministic transit prefix are admissible; foreign,
duplicate and wrong-length prefixes refuse before writes. The native NAT pool
must see negotiated IPv4 first. A correct pair is strictly no-write on repeated
polls. Interrupted exact subsets recover by additions or the same bounded
reordering; no delete-all, foreign writes, detached context or deadline increase.
Read back exact order and repeat process/NCP proofs before publishing readiness.
IPv6 and legacy PPP do not use this IPv4 promotion. Failures remain tracked for
mirror withdrawal and cannot publish forwarding readiness.

Focused race tests cover old order, no-write repetition, all interrupted subsets,
foreign-prefix/native-identity/tag refusal and ambiguous replies/cancellation at
every mutation. First test execution exposed an uninitialized mirror map in the
direct preparation fixture; corrected initialization and fixed explicit fixture
mirror identity. Second focused run PASS, 1.631s. Full gate and actual dual-host
NAT/lifecycle acceptance are still required. The original .195 11-second recovery
failure remains unresolved.

RA supplemental immutable supplier10 was rerun using a TCG CPU without AVX,
AVX2 or SHA-NI, while production units and all original deadlines stayed unchanged.
This diagnostic FAILED readiness (EXIT2); publisher initialization stage6/stage19
reached the existing caller deadline. This CPU change slowed validation and is
not a product remedy or acceptance success. Private raw output is root0600 under
/tmp/ngfw-completion-evidence-20261011; public kernel certificate identifiers are
not republished. Canonical RA activation/recovery/API/browser acceptance remains
open; no task is marked Done from a supplier-only result.

Complete current PPP-related package race tests PASS: subsystems39.217s,
descriptors/pppoe1.431s, renderers/pppoe62.451s. Contract check PASS47s.
Draft PR228 opened; full unchanged hosted gate38125408678 in progress.
Fresh static CGO0 product agent c55 SHA907dd8d8b65ba8b556e26ada6d909b5f23c33ac093582b40cae62c6f66072a46.
Actual .195 full API attempt1 failed before product launch: fixed .5s fixture wait
observed ip-netns-exec before it became the genuine peer executable. All strict
process/name checks stayed enabled and cleanup/shared VPP1014/NRestarts0 and host
identity checks passed. Fixture now waits at most two seconds for that genuine
transition, matching its existing bounded restart guard; no fake identity.

Primary upstream ppp v2.5.2 plugin.c and discovery.c show PADI/PADR timeout doubling
(default initial5s), allowing the retained15.096s restart gap. Carrier-only peer
configuration now proposes initial2s/two attempts per discovery phase, so a cycle
is bounded and persist returns to the configured holdoff. Legacy peers, holdoff,
maxFail, authentication and the original11s acceptance remain unchanged. Actual
new-source peer-restart/NAT acceptance and fresh paired binary remain required.
Sources: https://github.com/ppp-project/ppp/blob/v2.5.2/pppd/plugins/pppoe/plugin.c
and https://github.com/ppp-project/ppp/blob/v2.5.2/pppd/plugins/pppoe/discovery.c .

Capacity remedy: no running Go tool used the default root build cache. Standard
GOCACHE=/root/.cache/go-build go clean -cache is releasing its22GiB generated
compilation artifacts; source/configuration/evidence and module cache retained.
