package vpn_test

import (
	"ngfw/agent/internal/descriptors/vpn"
	"ngfw/agent/internal/scheduler"
	"testing"
)

type replaySetter struct {
	setter
	enabled bool
}

func (s *replaySetter) CreatePreservesDependents() bool { return s.enabled }

func TestGlobalInPlaceReplayForwardedOnlyByOwner(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		inner := &replaySetter{enabled: enabled}
		owner := vpn.Global(true, inner, nil)
		forwarded, ok := owner.(scheduler.WriteOnlyInPlaceCreator)
		if !ok || forwarded.CreatePreservesDependents() != enabled {
			t.Fatal("owner lost explicit opt-in", enabled)
		}
		if _, ok := vpn.Global(false, inner, nil).(scheduler.WriteOnlyInPlaceCreator); ok {
			t.Fatal("non-owner requirement acquired setter privilege")
		}
	}
	owner := vpn.Global(true, &setter{}, nil).(scheduler.WriteOnlyInPlaceCreator)
	if owner.CreatePreservesDependents() {
		t.Fatal("ordinary global setter opted in")
	}
}
