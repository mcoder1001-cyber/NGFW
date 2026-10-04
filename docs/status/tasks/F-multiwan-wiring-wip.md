# F-multiwan-wiring WIP
Local/remote SHA: checkpoint publication pending.
Completed: audited integrated runtime and found PBR group selection can use IPv6 route for an IPv4 group sharing a member. Corrected selection by route table/address family; regression covers independent v4/v6 active members.
Tests: pending. Remaining: focused race tests, full quick gate, independent review, PR.
Next command: cd apps/agent && go test -race ./internal/multiwan -count=1.

Focused race PASS2.090s; independent root review APPROVE. Source closeout recorded in F-multiwan-wiring.md. Remote implementation5ecb349d5ed26fa02cbcb71d7ba046119df346a2 verified via connector; CLI403. Quick prerequisite fixture fixes live on root PR162, not waived. Next manager command: rebase159 after162 then complete unchanged quick.
