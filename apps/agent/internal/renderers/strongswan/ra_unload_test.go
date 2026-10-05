package strongswan

import (
	"context"
	"errors"
	"github.com/strongswan/govici/vici"
	"testing"
)

type raUnloadFake struct {
	raFake
	foreign, malformed, failed, retained bool
	unloaded                             bool
	calls                                []string
}

func (f *raUnloadFake) Call(_ context.Context, command string, _ *vici.Message) (*vici.Message, error) {
	f.calls = append(f.calls, command)
	switch command {
	case "get-conns":
		if f.malformed {
			return msg("conns", "invalid"), nil
		}
		if f.foreign {
			return msg("conns", []string{"foreign"}), nil
		}
		if f.unloaded && !f.retained {
			return msg("conns", []string{}), nil
		}
		return msg("conns", []string{"ra-road"}), nil
	case "get-pools":
		if f.unloaded && !f.retained {
			return msg(), nil
		}
		return msg("clients", msg("online", "0")), nil
	case "stats":
		return msg("ikesas", msg("total", "0")), nil
	case "unload-conn":
		if f.failed {
			return nil, errors.New("NGFW_TEST_REMOTE_ECHO")
		}
		f.unloaded = true
	}
	return msg("success", "yes"), nil
}
func TestRAUnloadRequiresOwnedCompleteReadback(t *testing.T) {
	for _, test := range []struct {
		name string
		f    raUnloadFake
		ok   bool
	}{{"owned", raUnloadFake{}, true}, {"foreign", raUnloadFake{foreign: true}, false}, {"malformed", raUnloadFake{malformed: true}, false}, {"daemon-error", raUnloadFake{failed: true}, false}, {"retained", raUnloadFake{retained: true}, false}} {
		t.Run(test.name, func(t *testing.T) {
			f := test.f
			err := UnloadRA(context.Background(), &f, "ra-road", []string{"clients"})
			if (err == nil) != test.ok {
				t.Fatal("unload result", err)
			}
			if test.name == "foreign" || test.name == "malformed" {
				for _, command := range f.calls {
					if command == "unload-conn" || command == "unload-pool" || command == "terminate" {
						t.Fatal("mutated unproved daemon")
					}
				}
			}
		})
	}
}
