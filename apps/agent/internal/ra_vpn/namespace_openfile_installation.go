package ravpn

import (
	"context"
	"os"
	"time"

	"golang.org/x/sys/unix"
	"ngfw/agent/internal/vpp/bootid"
)

// NumericPublisherValidationBudget bounds fresh whole-artifact installation
// validation independently of the IPC phase. Caller cancellation still applies.
const NumericPublisherValidationBudget = 20 * time.Second

// NumericPublisherIPCBudget bounds each manager exchange after installation
// validation. It does not permit an unbounded socket or process lifetime.
const NumericPublisherIPCBudget = 5 * time.Second

// NumericOpenFilePublicationBudget bounds the complete installation, identity,
// two fresh manager captures and publication operation. It is not a cache TTL.
const NumericOpenFilePublicationBudget = 40 * time.Second

// A proof belongs to exactly one call/process and holds the actual validated
// files. Every trust boundary must verify full file stamps and fresh canonical
// paths plus the complete creating process boot identity. No proof crosses a
// call, agent restart or helper execution. Close releases every held descriptor.
type numericPublisherInstallationProof struct {
	source bootid.Identity
	files  []numericPublisherHeldArtifact
}

type numericPublisherHeldArtifact struct {
	path       string
	file       *os.File
	stat       unix.Stat_t
	limit      int64
	executable bool
}

func newNumericPublisherInstallationProof(context.Context) (*numericPublisherInstallationProof, error) {
	return nil, ErrBoundary
}

func (*numericPublisherInstallationProof) Verify(context.Context) error { return ErrBoundary }
func (*numericPublisherInstallationProof) Close() error                 { return nil }
