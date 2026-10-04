# LDP bounded collision-probe debt

Owner: Codex manager. Due: 2026-10-11. Accepted by manager 2026-10-04.
The LDP source supports at most 256 distinct dynamic label routes. An oversized
read fails and preserves the last good snapshot through the 60-second hold-down.
This bounds work while the inherited DF-7 Create collision guard dumps the label
table for each new label. Ownership checks remain mandatory and unchanged.

Follow-up: design a scoped bulk-safe conflict/ownership snapshot retrieval seam,
reusing one authoritative VPP dump per reconciliation transaction with invalidation
under the service lock. Preserve static/SR/IP-binding collision refusal and
restart ownership records. Add large snapshot scaling regression before increasing
the support cap. The current cap is product behavior, not a throughput claim.
