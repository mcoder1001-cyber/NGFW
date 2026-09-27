# Reviewer R6 — UX / web / i18n   (prepend 00-CONTEXT.md, then ../REVIEW-PROMPT.md)

Mandatory when the diff touches `apps/web/**`, `packages/ui-kit/**`, locales, screenshots or `docs/user/**`. Read
`docs/05-ui-spec.md` first.

## Check
1. **UI honesty:** every new screen calls a real endpoint (grep the diff for TODO/mock/stub/fake data); loading, empty and error
   states exist; a screenshot of the real screen against a running slot stack is in the status file (T4 produces the full set).
2. **i18n:** no hard-coded user-visible strings in JSX/TSX; every key exists in **both** `en` and `fa` locales with a real Persian
   translation (not English copied); numbers, dates and units formatted through the i18n helpers; the UI never names the
   data-plane product — it is "the engine" / «موتور» (D-155).
3. **RTL and logical CSS:** `margin-inline-*`, `padding-inline-*`, `inset-inline-*`, `text-align: start/end` — `margin-left/right`,
   `left/right`, `float: left/right` → MAJOR; icons that imply direction mirror in RTL; screenshot in `fa` present.
4. **UX consistency:** the component library (`packages/ui-kit`) instead of new ad-hoc components; forms validate like the schema
   (same limits and messages); destructive actions confirm; keyboard access and labels (a11y basics).
5. User docs (`docs/user/**`) match the screen.

## Output
`docs/status/tasks/<id>-review-R6.md` — findings (with screenshot names), verdict line.
