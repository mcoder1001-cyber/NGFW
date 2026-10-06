# RA fixed publisher manager pair — unfinished

Branch: codex/ra-manager-pair-20261006. Base local54c8e142d69c0b93f611926ae64fcd752d6d4a73 / remote7dda37950a9221fb0c2053d29b4e623862b5dc64 / tree691b10374c5abcc5a075056f50335d29d2632c26.

Owned: new namespace_openfile_manager_pair.go/test.go; only replacement of two property queries in namespace_openfile_transport.go; this WIP/envelope. No Root target provider changes.

Contract first: one fresh fixed systemctl show --all query for publisher socket and service, with a closed property union and exact Id-bound blocks. Require each role's own mandatory subset; permit other union properties, preserve empty values; reject duplicates, missing/foreign units, malformed input and bounds violations. No cached snapshot, atomicity claim, fallback, skipped predicate or enlarged budget. Keep original two-second command context, caller deadline and one-second WaitDelay; existing proof and process checks remain.

Official systemd v259 implementation supports multiple units and separates their property blocks with blank lines: https://raw.githubusercontent.com/systemd/systemd/v259/src/systemctl/systemctl-show.c . Root actual readonly multi-unit query also confirmed empty values and role-dependent union fields. Fixed RA socket has one listener, so duplicate Listen is rejected.

Concrete overhead: publisher manager trust checks launch ten processes across probe and twelve across publish; pairing reduces these twenty-two to eleven, leaving peer and exit checks intact. Installation Verify reopens metadata, not a repeated full SHA scan. Actual Boot29 remains negative under original Connected30; no READY claim.

Actual checks: contract/type declaration only, no consumer yet. Remaining: strict parser/query, unchanged predicate integration, meaningful malformed/reordered/cancellation tests, whole RA race and contextual lint, independent reviews, actual guest replay. Next command: implement closed parser and fixed query.

## Consumer freeze

Contract local b152b5276b5d94264a010b9be554e63cfe1ff546, remote ccf35bd6a0e16d0aceae0d1f7de50a55f1fcf40e, exact tree4e05d1389f43328462cc2cbde65a9769a2523410 published successfully before implementation.

Implemented fixed two-unit command, --all, Id and closed union; original two-second caller-clipped deadline and one-second WaitDelay. Output bounded16384 bytes, validUTF8/noNUL/CR, exactly two separated blocks, role-specific mandatory properties, all duplicate/unknown keys and missing/duplicate/foreign unit identities refused. Empty values retained. Any command/parser ambiguity redacts to existing fixed stage6; unchanged socket/service predicate failures retain6/7. No fallback. All before/after installation proof and live fullboot/caps/cgroup/image checks retained. Generic Root target query helper unchanged.

Actual final whole RA race EXIT0 2.829s: /root/rct/rc-manager-pair-race-final.log. Contextual unchanged golangci-lint EXIT0, zero issues: /root/rct/rc-manager-pair-lint.log. Tests cover reversed unit order/Id last, empty/role-extra properties, duplicate keys/IDs, missing/foreign units, malformed/merged/extra blocks, unknown/non-property lines, NUL/CR/nonUTF8/oversize, cross-role missing properties, unchanged foreign-fragment/live-as-inactive refusal and already-cancelled caller with exact fixed argv/reap bound. Initial test-file write used wrong working directory and failed before mutation; corrected file was included in final whole-package replay. Earlier pre-test replay3.561s is not the final consumer test evidence.

No privileged execution or READY grade. Remaining independent source review and actual immutable canonical guest replay; existing Boot29 failure remains preserved. Next command after independent approval: Root integrates these exact owned five paths; build fresh coherent VM bundle from Root integration source.
