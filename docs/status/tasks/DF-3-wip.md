# DF-3 — WIP (continuing worker, slot 9 / w9)

Updated: 2026-09-24 00:40

| Family | Descriptors | Unit (fake) | Host integration | Doc |
|---|---|---|---|---|
| nat44-ed | done | green | green (00:19 run) | done |
| nat44-ei | done | green | re-run pending (the VPP crash cut the 00:19 run) | done |
| nat64 | done | green | re-run pending (same) | done |
| nat66 | done | green | green | done |
| det44 | done; enable no longer disables the plugin (VPP crash, questions Q0) | green | re-run pending | update pending |
| map (pkg `mapnat`) | salvaged; interface key now `<if>/<map-e\|map-t>` | todo | todo | todo |
| cnat | todo | | | |
| pnat | todo | | | |

Notes:
- The det44 test crashed the shared VPP (see DF-3-questions.md Q0). It is fixed and the fix is committed before any
  further host run.
- Scope follows the envelope: npt66 and dslite are not built (Q5).
