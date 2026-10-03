# P10 socket ownership and packaged capability diagnosis

Inspected exact main78218e6da8b623debf2542bebbe15af2fcc5a809 on 2026-10-03 UTC.
**Default socket setup does not establish a CAP_CHOWN requirement.** The daemon-file
ownership mismatch remains a distinct concrete source gap already recorded in
PENDING-P10-agent-file-ownership.md; it must not be justified by misclassifying
same-group socket chown as arbitrary UID transfer.

## Exact source chain

- deploy/debian/vrx/debian/vrx-agent.postinst:4–6 creates vrx group/user when absent,
  and root:vrx0750 persistent agent/config directories. No startup socket is installed.
- deploy/systemd/vrx-agent.service:9–16 sets User=root, Group=vrx,
  VRX_SOCKET_GROUP=vrx, optional /etc/vrx/agent.env. Lines18–22 request runtime
  directory vrx0750 and persistent vrx/agent0750; line36 retains ONLY
  CAP_NET_ADMIN CAP_SYS_ADMIN CAP_IPC_LOCK; /run/vrx remains writable (line38).
- apps/agent/internal/agent/agent.go:103–104 defaults socket /run/vrx/agent.sock and
  group vrx; lines272–275 propagate listenUnix failure, close VPP connection and
  abort agent creation before gRPC setup. Unit Restart=always then retries.
- apps/agent/internal/agent/server.go:157–199 creates parent0750 (does not reset an
  existing parent's permissions/ownership), rejects non-socket/active socket,
  binds new Unix socket, resolves configured group or warns/falls back to Getgid,
  calls os.Chown(path,-1,gid) at191 and Chmod0660 at195. Newly created socket owner
  is process UID; uid=-1 preserves that owner.
- deploy/systemd/vrx-api.service:9–10 uses vrx:vrx. Default root-owned vrx-group
  socket0660 and directory0750 permit the API group to traverse/connect.
- deploy/debian/vrx/assets/firstboot.sh:24–30 provisions other private files and
  chowns its secret to vrx. deploy/systemd/vrx-firstboot.service:7–13 is a separate
  oneshot without the agent's CapabilityBoundingSet; its chown cannot demonstrate
  that the bounded agent can change arbitrary owners. It does not precreate agent.sock.

Linux chown(2), https://www.man7.org/linux/man-pages/man2/chown.2.html (checked
2026-10-03): owner changes require CAP_CHOWN; a file owner may assign a member
 group without that capability; uid=-1 preserves UID. Therefore root process
with effective group vrx can request the socket's vrx group without CAP_CHOWN.
This is a source/kernel-contract inference, not an executed packaged-unit test.

## Concrete failure triggers

1. Set VRX_SOCKET_GROUP via agent.env to an existing group outside process effective/
   supplementary membership, retain the three-capability bound, and reach listenUnix
   after earlier VPP/service prerequisites succeed. Group lookup succeeds; arbitrary
   GID transfer lacks CAP_CHOWN and can return EPERM at server.go191. The function
   closes listener and startup propagates `chown socket`; gRPC does not start. Missing
   groups instead warn/fall back, so a nonexistent group alone is not this trigger.
   Even granting CAP_CHOWN would not automatically give the API the selected group.
2. Existing default daemon renderer ownership is different: helpers_files.go24 creates
   agent-owned new temporary file,43 calls chown,90–98 resolves File.Owner then
   os.Lchown to another UID/group. frr/paths.go52 chooses frr:frr;
   kea/paths.go98 chooses _kea:_kea; chrony/paths.go73,100 choose _chrony:_chrony
   for keys (config root:_chrony at71,98). Different UID assignment cannot be solved
   by same-group socket reasoning or preowned destination, since every atomic write
   creates a new temporary inode. Current three-capability agent cannot perform this
   UID transfer; permission failure aborts file install. Existing PENDING remains valid.
3. Missing/foreign parent access, conflicting non-socket path or active socket are
   separate earlier failures; adding CAP_CHOWN is not an evidenced remedy for them.

## Minimal bounded options, pending independent R7

- Recommended socket scope: retain packaged privileges and default group. Add a
  source-tested explicit configuration check for an existing requested group outside
  process memberships, with clear fail-closed diagnostic; preserve nonexistent-group
  fallback unless separately reviewing its contract. Optionally avoid redundant chown
  only after verifying actual freshly bound inode UID/GID; do not skip a needed group
  change or weaken0660. No new capability is required for the default socket contract.
- Arbitrary socket-group support would require explicit service supplementary-group
  provisioning and matching API group/parent traversal contract; it is not a generic
  reason to grant arbitrary CAP_CHOWN. A capability addition remains privilege review.
- For daemon UID transfer, separately review existing PENDING options: CAP_CHOWN,
  narrow authenticated privileged file helper, or redesigned renderer/daemon ownership.
  No option is selected/implemented here. Broad writable /etc remains a separate gap.

## Meaningful planned source regression (NOT RUN)

Inject group lookup/Getgid/Getgroups and chown observation/errors into a bounded
socket helper test seam, with real listener in private test directory: matching
primary group succeeds without privilege request; permitted supplementary group
sets requested GID; existing nonmember group refuses with actionable diagnostic;
missing group follows unchanged warning/fallback; injected EPERM closes listener and
returns error, while non-socket/active paths remain protected. Verify mode0660 and
unchanged UID. A renderer control should inject different UID ownership failure and
prove atomic write preserves destination/cleans temporary file. These controls test
actual decisions/failure propagation, not string presence or mirrors of implementation.
Later dedicated isolated appliance test must execute the packaged capability set and
API connectivity; NOT RUN here and not waived as source acceptance.

Read root AGENTS, context/contributing/decision policy, P10 prompt/envelope,
PENDING ownership report, unit-agent independent review and resume R8 report.
Only source inspection and docs whitespace checks executed. No root/syscall mutation,
unit execution, heavy Go, target test or new product PASS is claimed.

## Historical evidence comparison

No prior reviewed report inspected here says the default same-group socket chown
requires CAP_CHOWN. P10-unit-agent-review.md:20 explicitly attributes the missing
capability to renderer temporary files/fixed daemon owners; P10-unit-packaging-review.md:28
and P10-packaging-resume-review-R8.md:14 preserve that same daemon boundary.
P10-code-report.md:47 records Unix socket listen EPERM AND foreign-owner chown EINVAL
as local validation-environment failures, not an executed installed-unit socket
CAP_CHOWN diagnosis. P10-packaging-finish-wip.md:81 similarly records temporary
fake-agent socket listen failures. Those historical failures remain valid observations;
relabeling them as default packaged socket ownership failure would contradict their
limited execution scope. This report corrects that potential inference without rewriting
history or claiming an actual default appliance PASS.
