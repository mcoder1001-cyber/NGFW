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
