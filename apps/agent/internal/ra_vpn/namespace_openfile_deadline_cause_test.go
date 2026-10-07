package ravpn

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// Reproduce the observed client deadline/server cancellation distinction with
// actual socket watchers and an unchanged healthy held proof. Expiry is supplied
// explicitly, so this test does not depend on emulation speed or setup latency.
func TestNumericPublisherCallerExpiryCancelsHealthyServerProof(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root-owned held artifact fixture requires root")
	}
	proof := proofDiagnosticFixture(t)
	if line, err := captureProofDiagnostic(context.Background(), proof); err != nil || line != "" {
		t.Fatal("healthy installation proof refused or logged", err)
	}
	pair, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_SEQPACKET|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, fd := range pair {
			if err := unix.Close(fd); err != nil {
				t.Error(err)
			}
		}
	})
	server, cancelServer := context.WithTimeout(context.Background(), NumericPublisherServerWholeBudget)
	defer cancelServer()
	joinServer := watchNumericPublisherPeer(server, pair[1], cancelServer)
	defer joinServer()
	work, cancelWork := context.WithTimeout(server, NumericPublisherWorkBudget)
	defer cancelWork()
	// Model setup consuming the caller budget before this boundary. The real
	// publication40 child must inherit expiry instead of granting another40.
	caller, cancelCaller := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancelCaller()
	publication, cancelPublication := context.WithTimeout(caller, NumericOpenFilePublicationBudget)
	defer cancelPublication()
	if publication.Err() != context.DeadlineExceeded {
		t.Fatal("publication widened expired caller budget")
	}
	joinClient := watchNumericPublisherCancellation(publication, pair[0])
	defer joinClient()
	select {
	case <-work.Done():
	case <-time.After(time.Second):
		t.Fatal("caller socket shutdown did not cancel server work")
	}
	if server.Err() != context.Canceled || work.Err() != context.Canceled {
		t.Fatal("peer shutdown was incorrectly attributed to server deadline")
	}
	line, verifyErr := captureProofDiagnostic(work, proof)
	want := numericPublisherProofFailureLine(numericPublisherProofEntry, 0, context.Canceled)
	if verifyErr != ErrBoundary || !strings.HasSuffix(line, want+"\n") || strings.Count(line, "remote-access publisher-proof ") != 1 {
		t.Fatal("healthy proof did not refuse canceled peer context", verifyErr, line)
	}
	for _, artifact := range proof.files {
		if _, err := artifact.file.Stat(); err != nil {
			t.Fatal("failure closed caller-owned held artifact", err)
		}
	}
	if line, err := captureProofDiagnostic(context.Background(), proof); err != nil || line != "" {
		t.Fatal("caller expiry altered installation or poisoned proof", err)
	}
}
