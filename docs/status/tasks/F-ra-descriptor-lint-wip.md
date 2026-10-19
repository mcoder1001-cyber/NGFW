# RA descriptor lint checkpoint

Branch codex/ra-descriptor-lint-20261005. Base engine67c055/local32500172e; three controller files copied exactly from supplier local4bbd38 before edits, including its newer descriptor test assertions. Explicit ownership: engine_descriptor.go, engine_descriptor_test.go, unit_supervisor_test.go authorized by controller; tap_recovery.go authorized by engine.

Changes: explicit documented read-only FD close acknowledgement; executable/unsafe-mode negative fixture annotations preserve exact modes and assertions; checked test descriptor closes. TAP absence now refuses recovery if dump stream Close reports failure, preserving all foreign/old index/namespace and bounded dump checks.

Actual complete RA race PASS2.051s. First lint exited3 due to another running lint lock, retained. Retry exited1:29 findings elsewhere, zero findings in these four owned files. Whole lint remains FAIL. Logs /root/ngfw-observer-tmp-20261005/ra-descriptor-race.log and ra-descriptor-lint-retry.log.

Remaining: independent R1/R2/R4 source review and concrete latest coherent integration. No actual private VPP stream failure or privileged fixture replay claim. Next command: consume only these four exact paths into latest supplier source, retaining current helper/controller contracts; independent whole unchanged quick remains required.
