# F-images: independent R1 review

Frozen local source bf9fa4b2. Complete unchanged quick gate runs in isolated detached /root/ngfw-wt/r1-gate-images-20261005. No developer worktree edits.

Source correctness review: signed pool and pinned VPP manifest reuse existing P14 verifier; exact shared partition contract plus BIOS boot partition; target chroot/nspawn-only package and GRUB operations; private mount namespace and reverse cleanup; configuration mutation preflight; clone identity/password checks; explicit management NIC; format metadata/check/byte comparison; SHA256 manifest. Eleven Python tests cover layout boundaries, symlink escape atomic refusal, completion marker, profiles/identity, credential/package rejection and real small format roundtrips. The Go topology module invokes these in unchanged quick CI.

No source correctness finding identified. Full appliance build/offline mounted inspection requires signed dependency pool, manifest and build disk headroom; production image artifacts are NOT proved by 16MiB conversions. VM/cloud boot acceptance is explicitly deferred and must remain in the central ledger, with no release artifact claim.

Independent full unchanged gate completed CI GATE PASSED in27m57s at exactbf9fa4b25a62c4009229ab23c9d0a4480e507953; final working tree clean. Output is pasted in F-images-test-T1.md. No source correctness findings.

Verdict: APPROVE (production build/boot acceptance explicitly remains deferred; no appliance artifact claim).


Superseding corrected tree: 6bd58ecbe10efb45a486139777655c6c826093cd, remote3a1b3e37de14939ac0ed5d8e66e17b08f05ea857, root integration branch. Fresh R4 identified GRUB command host-root/alias guard gap on oldbf9 tree; source repaired by root, not this reviewer. Independent targeted inspection confirms shared mutation_root used by put/configure/grub_config, rejecting /, noncanonical root aliases/ancestors and GRUB write-path symlinks before kernel reads or writes. New refusal cases preserve external sentinel contents. No remaining finding identified; original approval/fullgate applies only bf9. Corrected tree final R1 approval awaits full quick gate.

Independent actual command in source-only /root/ngfw-wt/r1-gate-images-20261005 at exact6bd: `TMPDIR=/root/.cache/review-r1-tmp tools/heavy.sh python3 -m unittest discover -s test/topology/images -p test_images.py -v`, thirteen PASS3.705s, tracked status clean. Raw log images-root-fix-focused.log. New exact-tree full unchanged T1 queued when root active gates release scarce disk/temp space; previous fullgate not represented as passing the source change.


Final corrected-tree verdict: APPROVE on6bd58ecbe10efb45a486139777655c6c826093cd / tree8d9829d754b58b40842ebff74a30641acda1dee9. Fresh R4 GRUB-root finding resolved and independently13tests verified. Own unchanged complete quick PASS22m44s, all checks including real generation clean and final full status empty. No remaining correctness finding identified. Supersedes provisional corrected-tree pending statement; oldbf9 results remain historical evidence only.
