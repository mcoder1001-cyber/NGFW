# TD-3 WIP — V19 guard (D-095) — fix round 1 (after BLOCK 46fafdb)

Slot 2 (w2). Branch task/TD-3. Respawned 07:2x; salvage commit d5116b8 reviewed and kept.

| finding | state |
|---|---|
| H1 L3-mode reset + resurrect + quarantine (Acquire) | code done (salvage) + resurrect made exact for input ACL, FreshRun=8; unit tests done; host test TODO |
| H2 no blind spot (resurrect before probe) | code + unit done |
| H3 BeforeDelete in every interface Delete (+ lcp) | wired (salvage); verify |
| M1 classify table delete refusal for stale-index records | TODO |
| M3 preflight before-tests under exclusive lock, after-tests run, exit 2 message | TODO |
| M4 preflight: quarantine = WARN | TODO |
| L1 per-phase metrics + quarantine gauge | done |
| docs + status fix-round section + CI | TODO |
