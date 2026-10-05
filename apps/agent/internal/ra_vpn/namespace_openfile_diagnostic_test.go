package ravpn

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNumericPublisherFailureDeadlineAndBoundary(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	err := numericPublisherFailure(ctx, 6)
	var diagnostic *NumericPublisherFailure
	if !errors.As(err, &diagnostic) || diagnostic.Stage != 6 || !diagnostic.DeadlineExceeded || !errors.Is(err, ErrBoundary) {
		t.Fatal("bounded deadline classification lost")
	}
	if strings.Contains(err.Error(), "context deadline") || err.Error() != "remote-access publisher refused at stage 6 deadline=true" {
		t.Fatal("unexpected diagnostic text")
	}
	live := numericPublisherFailure(context.Background(), 2).(*NumericPublisherFailure)
	if live.DeadlineExceeded {
		t.Fatal("live context reported deadline")
	}
}
