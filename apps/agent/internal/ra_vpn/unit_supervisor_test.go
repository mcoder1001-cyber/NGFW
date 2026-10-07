package ravpn

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestUnitObservationRequiredBeforeReadiness(t *testing.T) {
	if (SystemdUnits{}).Preflight(context.Background()) == nil {
		t.Fatal("missing manager observer advertised ready")
	}
}

func TestUnitSnapshotCloseOwnsBothDescriptors(t *testing.T) {
	first, err := os.Open("/dev/null")
	if err != nil {
		t.Fatal(err)
	}
	second, err := os.Open("/dev/null")
	if err != nil {
		t.Fatal(err)
	}
	snapshot := &UnitProcessSnapshot{Network: first, Executable: second}
	if err := snapshot.Close(); err != nil {
		t.Fatal(err)
	}
	if snapshot.Network != nil || snapshot.Executable != nil {
		t.Fatal("closed descriptors retained")
	}
	if _, err := first.Stat(); err == nil {
		t.Fatal("network descriptor remained open")
	}
	if _, err := second.Stat(); err == nil {
		t.Fatal("executable descriptor remained open")
	}
	if err := snapshot.Close(); err != nil {
		t.Fatal("idempotent close failed")
	}
}

func TestUnitSnapshotRejectsProcLikeRegularDescriptor(t *testing.T) {
	_, verifier, _, units, _, _, spec := lifecycleFixture(t)
	file, err := os.Open("/dev/null")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Error("fixture descriptor close failed")
		}
	}()
	snapshot := &UnitProcessSnapshot{Instance: spec.Instance, Identity: units.id, ControlGroup: "/system.slice/ngfw-ra@" + spec.Instance + ".service", Network: file, Executable: file}
	if _, err := verifyUnitSnapshot(verifier.plan, snapshot, true); err == nil {
		t.Fatal("ordinary descriptor accepted as private NETNS")
	}
	snapshot.Instance = "foreign"
	if _, err := verifyUnitSnapshot(verifier.plan, snapshot, true); err == nil {
		t.Fatal("foreign instance accepted")
	}
}

func TestUnitExecutableRequiresExactOwnedInode(t *testing.T) {
	dir := t.TempDir()
	owned := dir + "/owned"
	other := dir + "/other"
	// #nosec G306 -- executable fixture requires owner execute permission; no group or other access.
	if err := os.WriteFile(owned, []byte("same bytes"), 0700); err != nil {
		t.Fatal(err)
	}
	// #nosec G306 -- foreign executable fixture has the same owner-only mode to test exact inode authentication.
	if err := os.WriteFile(other, []byte("same bytes"), 0700); err != nil {
		t.Fatal(err)
	}
	// #nosec G304 -- owned is the fixed executable fixture inside this test's private TempDir.
	file, err := os.Open(owned)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Error("fixture descriptor close failed")
		}
	}()
	if os.Geteuid() != 0 {
		if sameUnitExecutable(file, owned) {
			t.Fatal("non-root artifact authenticated as root-owned executable")
		}
		t.Log("non-root ownership refusal verified; positive root-owned inode assertions require UID 0")
		return
	}
	if !sameUnitExecutable(file, owned) {
		t.Fatal("exact root-owned executable refused")
	}
	if sameUnitExecutable(file, other) {
		t.Fatal("same bytes with foreign inode adopted")
	}
	// #nosec G302 -- deliberately unsafe mode must be refused by exact executable ownership validation.
	if err := os.Chmod(owned, 0777); err != nil {
		t.Fatal(err)
	}
	if sameUnitExecutable(file, owned) {
		t.Fatal("writable executable adopted")
	}
}

func TestUnitStartWithoutManagerProofDoesNotDispatch(t *testing.T) {
	_, verifier, _, _, _, _, _ := lifecycleFixture(t)
	id, err := (SystemdUnits{}).Start(context.Background(), verifier.plan)
	if err == nil || id.Valid() {
		t.Fatal("start without installed observation boundary accepted")
	}
}

type managerObserverProbe struct{ failure bool }

func (p managerObserverProbe) Acquire(context.Context, string) (*UnitProcessSnapshot, error) {
	return nil, ErrEngine
}
func (p managerObserverProbe) Preflight(context.Context) error {
	if p.failure {
		return ErrEngine
	}
	return nil
}

func TestManagerDispatchRejectsForeignOperationsBeforeCallback(t *testing.T) {
	calls := 0
	units, err := NewSystemdUnitsForManager(managerObserverProbe{}, func(_ context.Context, op UnitOperation, instance string) (UnitManagerState, error) {
		calls++
		if op != UnitOperationObserve || !ValidInstance(instance) {
			t.Fatal("unbounded manager dispatch")
		}
		return UnitManagerState{MainPID: 123}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"restart", "ngfw-ra@" + strings.Repeat("a", 64) + ".service"}, {"show", "--property=MainPID", "--value", "vpp.service"}, {"start", "ngfw-ra@../foreign.service"}} {
		if _, err := units.execute(context.Background(), args...); err == nil {
			t.Fatal("foreign operation dispatched")
		}
	}
	if calls != 0 {
		t.Fatal("invalid request reached manager")
	}
	pid, err := units.pid(context.Background(), "ngfw-ra@"+strings.Repeat("a", 64)+".service")
	if err != nil || pid != 123 || calls != 1 {
		t.Fatal("actual manager readback not preserved")
	}
}

func TestManagerDispatchDoesNotStartWithoutObservationPreflight(t *testing.T) {
	_, verifier, _, _, _, _, _ := lifecycleFixture(t)
	calls := 0
	units, err := NewSystemdUnitsForManager(managerObserverProbe{failure: true}, func(context.Context, UnitOperation, string) (UnitManagerState, error) {
		calls++
		return UnitManagerState{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if id, err := units.Start(context.Background(), verifier.plan); err == nil || id.Valid() {
		t.Fatal("missing observation proof accepted")
	}
	if calls != 0 {
		t.Fatal("start or readback dispatched before observation proof")
	}
	if _, err := NewSystemdUnitsForManager(nil, units.manager); err == nil {
		t.Fatal("nil observer accepted")
	}
	if _, err := NewSystemdUnitsForManager(managerObserverProbe{}, nil); err == nil {
		t.Fatal("nil dispatcher accepted")
	}
}

type activationObserverProbe struct {
	events         *[]string
	activationFail bool
	acquisitions   int
}

func (p *activationObserverProbe) Preflight(context.Context) error {
	*p.events = append(*p.events, "preflight")
	return nil
}
func (p *activationObserverProbe) PrepareObservation(_ context.Context, instance string) error {
	if !ValidInstance(instance) {
		return ErrEngine
	}
	*p.events = append(*p.events, "activate")
	if p.activationFail {
		return ErrEngine
	}
	return nil
}
func (p *activationObserverProbe) Acquire(context.Context, string) (*UnitProcessSnapshot, error) {
	p.acquisitions++
	*p.events = append(*p.events, "acquire")
	return nil, ErrEngine
}

func TestObserverActivationFailureRetainsUnobservedLaunch(t *testing.T) {
	_, verifier, _, _, _, _, _ := lifecycleFixture(t)
	events := []string{}
	observer := &activationObserverProbe{events: &events, activationFail: true}
	launched := false
	units, err := NewSystemdUnitsForManager(observer, func(_ context.Context, operation UnitOperation, _ string) (UnitManagerState, error) {
		switch operation {
		case UnitOperationObserve:
			if launched {
				return UnitManagerState{MainPID: 123}, nil
			}
			return UnitManagerState{}, nil
		case UnitOperationStart:
			launched = true
			events = append(events, "start")
			return UnitManagerState{}, nil
		default:
			t.Fatal("uncertain launch stopped without process proof")
		}
		return UnitManagerState{}, ErrEngine
	})
	if err != nil {
		t.Fatal(err)
	}
	id, err := units.Start(context.Background(), verifier.plan)
	if err == nil || id.Valid() || !launched || observer.acquisitions != 0 {
		t.Fatal("activation failure invented process proof or lost ambiguous launch")
	}
	if strings.Join(events, ",") != "preflight,start,activate" {
		t.Fatal("observation socket activation not strictly after daemon launch", events)
	}
}

func TestFailedStartActivatesObserverBeforePartialCapture(t *testing.T) {
	_, verifier, _, _, _, _, _ := lifecycleFixture(t)
	events := []string{}
	observer := &activationObserverProbe{events: &events}
	launched := false
	units, err := NewSystemdUnitsForManager(observer, func(_ context.Context, operation UnitOperation, _ string) (UnitManagerState, error) {
		switch operation {
		case UnitOperationObserve:
			if launched {
				return UnitManagerState{MainPID: 123}, nil
			}
			return UnitManagerState{}, nil
		case UnitOperationStart:
			launched = true
			events = append(events, "start-failed")
			return UnitManagerState{}, ErrEngine
		default:
			t.Fatal("partial launch stopped without identity proof")
		}
		return UnitManagerState{}, ErrEngine
	})
	if err != nil {
		t.Fatal(err)
	}
	id, err := units.Start(context.Background(), verifier.plan)
	if err == nil || id.Valid() || !launched || observer.acquisitions != 1 {
		t.Fatal("failed-start capture behavior incorrect")
	}
	if strings.Join(events, ",") != "preflight,start-failed,activate,acquire" {
		t.Fatal("partial capture ran before activation", events)
	}
}

type budgetObserver struct {
	check func(context.Context)
	calls int
}

func (o *budgetObserver) Preflight(context.Context) error { return nil }
func (o *budgetObserver) Acquire(context.Context, string) (*UnitProcessSnapshot, error) {
	return nil, ErrEngine
}
func (o *budgetObserver) PrepareObservation(ctx context.Context, _ string) error {
	o.calls++
	o.check(ctx)
	return ErrEngine
}

func TestObservationBootstrapKeepsFreshProofBudgetAndCallerCancellation(t *testing.T) {
	_, verifier, _, _, _, _, _ := lifecycleFixture(t)
	for _, short := range []bool{false, true} {
		t.Run(fmt.Sprint(short), func(t *testing.T) {
			ctx := context.Background()
			var want time.Time
			if short {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, time.Second)
				defer cancel()
				want, _ = ctx.Deadline()
			}
			observer := &budgetObserver{check: func(got context.Context) {
				deadline, ok := got.Deadline()
				if !ok {
					t.Fatal("unbounded bootstrap")
				}
				if short {
					if !deadline.Equal(want) {
						t.Fatal("caller deadline widened")
					}
				} else if left := time.Until(deadline); left <= NumericPublisherValidationBudget || left > NumericOpenFilePublicationBudget {
					t.Fatal("proof budget clamped or widened", left)
				}
			}}
			if (SystemdUnits{Observation: observer}).prepareObservation(ctx, verifier.plan) != ErrEngine || observer.calls != 1 {
				t.Fatal("bootstrap failure lost")
			}
		})
	}
}

func TestFailedStartCompensationAllowsFreshProofButCannotInventOwnership(t *testing.T) {
	_, verifier, _, _, _, _, _ := lifecycleFixture(t)
	observer := &budgetObserver{check: func(ctx context.Context) {
		deadline, ok := ctx.Deadline()
		left := time.Until(deadline)
		if !ok || left <= NumericPublisherValidationBudget || left > NumericOpenFilePublicationBudget {
			t.Fatal("failed launch proof budget", left)
		}
	}}
	calls := 0
	units, err := NewSystemdUnitsForManager(observer, func(_ context.Context, op UnitOperation, _ string) (UnitManagerState, error) {
		calls++
		if op == UnitOperationObserve {
			return UnitManagerState{}, nil
		}
		if op == UnitOperationStart {
			return UnitManagerState{}, ErrEngine
		}
		t.Fatal("unverified compensation dispatched stop")
		return UnitManagerState{}, ErrEngine
	})
	if err != nil {
		t.Fatal(err)
	}
	id, err := units.Start(context.Background(), verifier.plan)
	if err != ErrEngine || id.Valid() || calls != 2 || observer.calls != 1 {
		t.Fatal("failed bootstrap adopted/stopped unknown unit", id, err, calls, observer.calls)
	}
}
