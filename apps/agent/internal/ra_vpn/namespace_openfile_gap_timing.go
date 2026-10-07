package ravpn

import (
	"context"
	"fmt"
	"log"
	"time"
)

// This call-local trace supplements the frozen client/proof diagnostics. Only
// bounded elapsed values and remaining caller budget enter failure output.
type numericPublisherGapTiming struct {
	started time.Time
	budget  int64
	marks   [5]int64
}

func numericPublisherRemainingBudget(started, deadline time.Time, present bool) int64 {
	if !present {
		return NumericOpenFilePublicationBudget.Milliseconds()
	}
	return min(max(deadline.Sub(started).Milliseconds(), 0), NumericOpenFilePublicationBudget.Milliseconds())
}

func newNumericPublisherGapTiming(ctx context.Context, started time.Time) *numericPublisherGapTiming {
	deadline, present := ctx.Deadline()
	trace := &numericPublisherGapTiming{started: started, budget: numericPublisherRemainingBudget(started, deadline, present)}
	for index := range trace.marks {
		trace.marks[index] = -1
	}
	return trace
}

func (trace *numericPublisherGapTiming) mark(index int) {
	if index >= 0 && index < len(trace.marks) {
		trace.marks[index] = min(max(time.Since(trace.started).Milliseconds(), 0), NumericOpenFilePublicationBudget.Milliseconds())
	}
}

func (trace *numericPublisherGapTiming) failureLine(failed bool) string {
	if !failed {
		return ""
	}
	return fmt.Sprintf("remote-access publisher-client phase=completion entry_budget_ms=%d probe_response_ms=%d probe_verified_ms=%d probe_exit_ms=%d publish_response_ms=%d publish_verified_ms=%d", trace.budget, trace.marks[0], trace.marks[1], trace.marks[2], trace.marks[3], trace.marks[4])
}

func (trace *numericPublisherGapTiming) logFailure(failed bool) {
	if line := trace.failureLine(failed); line != "" {
		log.Print(line)
	}
}
