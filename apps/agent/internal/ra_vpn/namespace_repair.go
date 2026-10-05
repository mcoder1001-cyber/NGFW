package ravpn

import "context"

// NamespaceHandoffStoppedRepair repairs an owned export generation only after
// the shared runtime mutation guard has positively verified quiescence. The
// implementation must retain ambiguous or inaccessible old exports for recovery;
// it cannot adopt a current target by replacing an old ownership receipt.
type NamespaceHandoffStoppedRepair interface {
	ExportExistingRepair(context.Context, *NetworkPlan) error
}
