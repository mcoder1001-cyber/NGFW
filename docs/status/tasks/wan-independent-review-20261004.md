# Independent WAN review

APPROVE frozen WAN fb8bb4780960191aebdab2352c4bf5c48cf08feb,
tree 0ec6b009f965321b2431ad37f23116aad8626899 against 4c8d1b247.
APPROVE WAN integration delta 2fd82320a..3d914914adda0b1328ba1e3bf1020ef1cd072607,
integrated tree 6fc3fef89b93383e63900a5e89c422ac01b0fbec.
This is a focused independent source review, not certification of unrelated code
in the full integration tree. No blocking finding in the inspected delta.

Reviewed route source registration, separate persistent ownership keys, existing
FIB client-source conflict refusal, NAT descriptor instances and claim filtering.
WAN NAT requires explicitly enabled ED mode and yields both feature/address pool
to explicit configuration ownership. Instance creation refuses foreign existing
objects and configuration claims. The original output-feature and interface-
address Retrieve now require their own claim, preventing original descriptors
from consuming WAN instances on owned interfaces.

Reviewed PBR group expansion on detached desired state, all-down VRF lookup and
reference restoration: observed path/address/interface/VRF/weight must equal the
current expansion before saved group references replace runtime paths. Drift is
not blindly replaced with configured group labels. Static/default and duplicate
WAN route conflicts are rejected before forwarding projection, including while
all links are down. Unknown/stale probe generations grant no route.

Reviewed cleanup scope, generation and cancellation: dynamic non-static sessions
must match configured dead-member IPv4 addresses and member VRF tables. Cleanup
requires an observed healthy-to-unhealthy transition; unavailable local probe
policy does not create a cleanup transition. Configuration-pointer identity is
rechecked under the transaction lock immediately before mutation, closing the
commit/rollback observation race. Pending addresses/progress reset on generation
change. Cleanup calls have a two-second context, retain pending work on error,
limit page retention/deletions and resume deletion-shifted pages safely.

Scope qualification: the inherited NAT user-session reader drains the complete
VPP stream even after retaining its requested 513-row page. Thus the new limits
bound retained rows/deletions, not the number of wire details. The timeout bounds
operation duration; this report does not assert a strict wire-message budget.
The default probe adapter refuses non-default VRFs and network namespaces rather
than executing probes outside their configured scope. DHCP/PPPoE gateway handoff
is explicitly warned and omitted, not reported as implemented forwarding.

Evidence: inspected relevant descriptor/runtime/agent/schema changes plus tests
for route ownership instances, NAT instances, PBR drift, cleanup pagination,
healthy-down filtering and generation identity. Worker focused race/schema/guard
results are reported by the manager; reviewer did not rerun duplicate broad suites.
No product edit, VPP mutation or host privilege change performed.
