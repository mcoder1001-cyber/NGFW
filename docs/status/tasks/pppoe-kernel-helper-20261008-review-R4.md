# R4 PPP kernel helper boundary review

Verdict: APPROVE the scoped helper and inert unit source at local commit
`59ff96a4cb2078cbd59aa8ed3bb6aeffeb35b5ab`, tree
`618e0d799ad2e3dd518c9bbe6b1428f39680d71e`. This verdict does not approve
activation, complete PPP integration, or native packet acceptance. Publication
parity is outside this local source receipt.

## Reviewed boundary

The fixed broker must create persistent network namespace mounts visible to VPP.
Its intentional absence of mount namespace sandbox directives is consistent with
that requirement. A private mount namespace would need explicitly verified
outbound mount propagation or a separate host mount provisioner; adding the
ordinary private sandbox flags would conceal newly created mounts. The selected
fixed root oneshot is bounded by finite validated operations, root-controlled
request/result files, exact token/boot/generation identities, random nonce,
request digest, ten-second admission lifetime, twelve-second unit timeout and
control-group termination. Admission checks expiry again after acquiring the
carrier lock. No caller supplies executable paths or arbitrary commands.

The PPP child keeps a separate mount sandbox. Its carrier ledger is read-only,
its writable persistent state is restricted to its own token directory, and
temporary mounts hide unrelated `/etc`, `/var/lib` and `/run` content. This closes
the earlier finding that restricted UID 0 could forge a privileged root-owned
ledger through a writable bind. Starting PPP refuses a configured carrier until
the broker has withdrawn policy; the launcher takes an existing read-only lock.

The trusted launcher retains namespace-entry capabilities only until `setpriv`.
The fixed leaf checks effective/permitted/bounding capabilities equal only
NET_ADMIN and NET_RAW, zero inheritable/ambient capabilities, NoNewPrivileges,
and the current namespace's boot/generation identity before fixed PPP or probe
execution. SYS_ADMIN and SETPCAP cannot be regained by the descendant. The probe
binary has a root-controlled digest and is executed through its verified open
inode. Probe rules admit only bounded literal IPv4 targets for fixed protocols;
five-second sets and a locally assigned conntrack mark distinguish local probe
replies from forwarded flows. Cleanup restores normal policy and checks state;
ambiguous cleanup withdraws readiness and installs deny policy.

## Independent validation

At the exact source commit, `python3 scripts/tests/pppoe-kernel-carrier.py` passed
26 controls in 0.014 seconds. The unchanged packaging loader invocation
`python3 -m unittest deploy/debian/ngfw/tests/test_pppoe_carrier.py` passed the same
26 controls in 0.016 seconds. `git diff --check` passed before this receipt.
The controls use fake execution or isolated file fixtures; they cover capability,
namespace replacement, ownership, drift, partial failures, probe cleanup,
broker replay/expiry and unit asset boundaries. No host namespace, daemon,
sysctl, route, packet operation or aggregate CI was executed.

## Remaining integration risks and acceptance conditions

- A broker crash or lost/expired reply can follow a successful kernel mutation.
  The adapter must reject the reply, withdraw cached forwarding readiness and
  reconcile actual kernel/VPP inventory before retry or publication. File writes
  are atomic, but the kernel and ledger are not one transaction. Provisioning can
  leave an orphan that source correctly refuses to adopt silently. Unit timeout
  bounds execution; it is not a rollback guarantee.
- Parent integration must implement verified forwarding snapshots, discard stale
  generation observations, withdraw routes/readiness before fallible teardown,
  and require stopped PPP plus removed owned TAPs before namespace deletion.
  Logical interface policy identity, exclusive raw parent ownership, transit
  allocation and VPP rollback/readback remain separate review boundaries.
- NET_RAW permits Ethernet emission outside the inet nft boundary. If raw WAN
  plain-IP prohibition is enforced against a compromised PPP descendant, VPP must
  constrain the raw transport to PPPoE discovery/session EtherTypes. The helper
  nft policy alone proves no such Ethernet filtering.
- Existing IPv6 hooks must be reconciled with the PPP child's
  ProtectKernelTunables setting: privileged namespace sysctl changes belong in
  the broker, and a failed hook cannot be accepted as successful negotiation.
- Native acceptance must establish the installed systemd mount visibility,
  read-only binds and capability transition, actual nft/rule semantics, IPv4/IPv6
  NAT return through VPP and teardown/restart behavior. Asset text and fake tests
  do not establish those runtime properties.

No new helper-source blocker remains within the scoped boundary above.

## Final narrow delta recheck

APPROVE the scoped final helper source at
`50d3da442359bc00e17a1255c7e866f5e4ee78e6`, tree
`117a61932e3111b19bed628933b7b376a6752b25`. This supersedes the earlier local
source target while retaining all integration and native acceptance limits.

The delta permits MTU 128 through 1279 only as an IPv4 carrier: it configures and
verifies no IPv6 transit routes/addresses, explicitly disables IPv6 on transit
and PPP and sets namespace IPv6 forwarding to zero. IPv6 default-route policy is
rejected below 1280. Immutable IPv6 reservation still prevents future collision;
the negotiated PPP MTU must still exactly equal the configured carrier MTU.

Verified receipts now include sorted actual PPP CIDRs from the pinned namespace
address dump, excluding tentative and DAD-failed addresses. The parent must compare
the applicable negotiated hook addresses with this receipt before mirroring
readiness; a hook alone remains insufficient.

An inspect receipt may signal `repair_required` when one or both bound TAPs are
missing. It preserves the ledger and generation, reports configured false, rejects
foreign links, verifies the exact identity of any surviving TAP and rejects a
changed previously observed PPP index. Namespace boot/device/inode checks remain
in force. It performs no adoption or network mutation, and full verification
continues to fail. The parent must treat this as dependent teardown/recreation,
withdraw readiness first and allocate a fresh generation after cleanup.

Both service assets add `DevicePolicy=closed`; the PPP service permits only the
required `/dev/ppp` device in addition to systemd's standard device allowance.
This addresses root descendants' possible host block-device access independently
of the read-only filesystem and capability drop. Installed cgroup enforcement
remains part of native service acceptance.

Independent final direct helper controls passed 28 tests in 0.014 seconds; the
packaging loader passed 28 in 0.013 seconds. Two additional isolated fake controls
confirmed rejection of a replaced PPP interface index during missing-TAP repair
and exclusion of tentative/DAD-failed PPP addresses. `git diff --check` passed.
No native operations or CI were run.

The raw L2 boundary assumes the fixed packaged PPP daemon is trusted unless VPP
adds an EtherType filter. CAP_NET_RAW and inet nft policy do not establish
containment of malicious Ethernet emitted by a compromised PPP descendant; the
parent must preserve this explicit assumption in the final source contract.

## Installed private peer path correction

APPROVE the narrow source correction at
`1cc3c4ef44bc664e2de97b7ad7aa0c14c293e383`, tree
`67226a87b2614db5d55e2efd67386ad2bab292a0`. The tmpfiles private peer root and
the PPP child's read-only `/etc/ppp` bind source now agree on
`/var/lib/ngfw/agent/pppoe-carrier`. This lies inside the existing installed agent
ReadWritePaths allowance; no agent unit widening is introduced. Child ledger,
capability, device and broker boundaries remain unchanged. Independent packaging
loader passed 28 controls in 0.017 seconds.

The companion packaging regression at
`b46d65bcba741d8e94d25e2ac8b8961490531755`, tree
`4ff872770566041d6ed9300f8eac50828a634109`, verifies this against the actual
source or staged agent unit and keeps the real unit in the preparation fixture.
Independent asset and preparation fixtures passed seven tests in 0.966 seconds.
This validates source/staging agreement, not live systemd enforcement. All earlier
integration and native acceptance obligations remain.
