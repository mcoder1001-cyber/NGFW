package ravpn

import (
	"fmt"
	"log"
	"time"
)

// numericPublisherClientTiming is confined to one publication call. Fixed
// preflight and probe/publish milestones contain elapsed milliseconds only;
// unvisited milestones remain -1. No identities, paths, requests or errors enter
// the diagnostic representation. Failure logging occurs once, after work stops.
type numericPublisherClientTiming struct {
	started   time.Time
	preflight [4]int64
	exchange  [2][5]int64
}

func newNumericPublisherClientTiming() *numericPublisherClientTiming {
	trace := &numericPublisherClientTiming{started: time.Now()}
	for i := range trace.preflight {
		trace.preflight[i] = -1
	}
	for phase := range trace.exchange {
		for i := range trace.exchange[phase] {
			trace.exchange[phase][i] = -1
		}
	}
	trace.preflight[0] = 0
	return trace
}

func (trace *numericPublisherClientTiming) elapsed() int64 {
	return min(max(time.Since(trace.started).Milliseconds(), 0), NumericOpenFilePublicationBudget.Milliseconds())
}

func (trace *numericPublisherClientTiming) markPreflight(index int) {
	if index >= 0 && index < len(trace.preflight) {
		trace.preflight[index] = trace.elapsed()
	}
}

func (trace *numericPublisherClientTiming) markExchange(phase, index int) {
	if phase >= 0 && phase < len(trace.exchange) && index >= 0 && index < len(trace.exchange[phase]) {
		trace.exchange[phase][index] = trace.elapsed()
	}
}

func (trace *numericPublisherClientTiming) failureLines(failed, callerDeadline bool) []string {
	if !failed {
		return nil
	}
	lines := []string{fmt.Sprintf("remote-access publisher-client phase=preflight caller_deadline_exceeded=%t enter_ms=%d proof_done_ms=%d peer_reference_done_ms=%d manager_done_ms=%d elapsed_ms=%d", callerDeadline, trace.preflight[0], trace.preflight[1], trace.preflight[2], trace.preflight[3], trace.elapsed())}
	for phase, name := range []string{"probe", "publish"} {
		values := trace.exchange[phase]
		lines = append(lines, fmt.Sprintf("remote-access publisher-client phase=%s before_connect_ms=%d after_connect_ms=%d ready_received_ms=%d ready_proof_done_ms=%d request_sent_ms=%d", name, values[0], values[1], values[2], values[3], values[4]))
	}
	return lines
}

func (trace *numericPublisherClientTiming) logFailure(failed, callerDeadline bool) {
	for _, line := range trace.failureLines(failed, callerDeadline) {
		log.Printf("%s", line)
	}
}
