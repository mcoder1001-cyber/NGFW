# Independent bounded NAT46 review, 2026-10-04

Reviewer: management_acceptance, separate worktree/branch; reviewed source read-only. Reviewed `/root/ngfw-wt/codex-closeout-nat46-driver` commits `760696e3502a5da997267609573af3eaf1732665` and corrective `3d001ee60de023ef32bc18df2c48db570f84ef0d`.

Verdict: APPROVE bounded source and final guarded live acceptance, subject to unchanged complete quick gate on integration tree. Review did not modify product code or rerun another agent's slot8.

Recovered Embedded/Project/serverOf behavior is identical to historical `50f2ceca` except the added foreign-family guard. The fallback recognizes only IPv6 servers whose bits64..95 are zero and whose last32bits equal the service IPv4. Those project to a /64 return-path key; arbitrary other server addresses retain the /128 shape and remain subject to the documented VPP reply limitation. This proof cannot be claimed as arbitrary IPv6 bidirectional NAT46 support.

Checked restoration/assembly preserves configured server addresses, refuses multiple embedded mappings sharing a /64, and rejects IPv4/IPv4-mapped foreign prefix families during assembly. Non-NAT46 shapes remain outside the assembled NAT46 view; the corrected test expects them to be ignored according to existing Assemble contract.

Two initial review blockers were corrected in `3d001ee60`:

- The new handoff helper could connect to shared /run/vpp without a private guard. Driver now requires disposable flag, root-owned regular non-symlink startup file with exact fulltest marker and mount namespace different from PID1 before making runtime directories/mutations. The Go helper repeats identity/namespace checks before connecting. It deletes only both exact untagged slot rig ports with peers quiesced, leaving Linux veths for agent ownership.
- An embedded NAT46 /64 could collide with a nat.map /64 even when IPv4 prefixes differ, because VPP's map.c adds/deletes domains in one shared IPv6 prefix LPM keyspace. The desired-state validator now refuses exact canonical projected-key collisions with MAP-E or MAP-T, with the NAT46 IPv6 pointer and no NAT46 projection. Tests cover canonical/noncanonical equal keys plus noncollision controls.

Driver hardening explicitly asserts host test PASS/noSKIP, applied/idempotent outcomes, exact3/3 ICMP after bounded warmup, exact TCP response bytes, replay within30seconds, and removal of owned MAP domain/both MAP-T features/server route after rollback. Initial strict live evidence shows3/3, exact payload before and after restart, translated IPv6 source, canonical recreation0.97s, rollback/cleanup, sharedNRestarts0. Final guarded-source rerun separately completed exit0 with the rebuilt source3d001ee60 agent; reviewed `closeout-nat46-evidence/review-final-live.txt` confirms strict packet/restart/duplicate/rollback/cleanup gates and unchanged sharedNRestarts0. Slot8 released.

Focused author race evidence at `closeout-nat46-evidence/review-fixed-tests.txt`: nat46 PASS1.115s, mapnat PASS1.144s, desired PASS35.963s. Root manager independently owns final aggregate gate/publication; remote HTTP403 publication block remains explicit.
