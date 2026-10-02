# TEST-traffic-A capture transaction validation — bounded source

Own isolated NGFW-traffic-capture-transaction on
`task/TEST-traffic-A-capture-transaction-20261002`, base corrected producere169.
No older approved source/branch is changed. Owned traffic-a source/tests and docs.

Actual authority: docs/lab/shared-host-rules §1 manager slot assignment; §1b shared
lab lock. tools/lab rig_side_up lines326–328 reuses existing namespace/veth by name.
It writes no lease/run/candidate/namespace inode owner binding. Current scenario
lease has task/slot/prefix/boot/expiry/id only and lstat/read path race. Names do not
prove ownership. No existing reviewed manager transaction issuer/lock binding.

Implement same-descriptor private lease/binding reads, canonical typed slot/run/
boot/expiry/candidate fixture validation, injected bounded read-only namespace/
device observations and pre/post stable snapshots. Source fixture binding format
is explicitly speculative consistency data, never authoritative lab attestation.
Live validator fails NOTIMPLEMENTED before reads/observations. No producer/run.py
activation, host locks, ip/netns/VPP/SSH or host writes. No public API/schema.

Options for future authority decision: manager provisioning must record actual
namespace handle identity and ifindex under protected run root while holding the
shared lab/slot transaction lock, bind candidate revision/digest+lease/run/boot,
and revoke before teardown. Reusing names needs explicit ownership reconciliation.
This is a source gap, not merely pending lab acceptance; worker cannot invent it.
Meaningful fixture tests: valid two-side consistency, stale/foreign/duplicate JSON,
lease renewal/revocation and observation identity changes during validation,
private directory/file ownership/alias/symlink/nonregular/byte bounds and expiry.
Proof flags remain false, whole composed executor/config causality unimplemented.

Implemented bounded inactive source validator: root0700 same directory descriptor;
owned0600 unaliased regular JSON files ≤8192bytes, NOFOLLOW/NONBLOCK/CLOEXEC; same
bounded bytes parsed with duplicate/nonfinite refusal and stable inode/time/size.
Exact task slot/boot/run/lease id + canonical UUID, explicit caller expected
candidate digest/revision, finite two-hour lease checked before/after observations.
Typed exact namespace/device and positive handle/ifindex/MAC identities compared
against speculative fixture binding then observed twice; two namespace aliases
refused. Protected snapshots rechecked to catch renewal/revocation/change.
Matching metadata is never ownership, and no real observer backend/issuer exists.
This is not atomic against a trusted-root adversary, a held lock or provenance.

Preserved398df351 ruling and ebddf4fc producer identity closure unchanged, alongside
inherited original BLOCK. Actual strict47PASS4.000s with zero failures/errors/
skips/expected failures/unexpected success. New seven tests include actual same-fd
mutation-before-parse, FIFO/symlink/hardlink/uid/mode/byte boundaries, forged lease,
wrong explicit candidate, duplicate JSON, observed identity changes, revocation,
expiry during read-only fixture observations and live refusal before host calls.
Unchanged sourced check EXIT0 in2s; no host/network/lock writes. Independent review
and hosted gate remain required; live producer/run.py is not enabled.
