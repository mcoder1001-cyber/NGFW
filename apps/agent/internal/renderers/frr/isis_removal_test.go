package frr

import (
	"context"
	"errors"
	"ngfw/agent/internal/renderers"
	"os"
	"strings"
	"testing"
)

func TestISISRemovalReloadErrorRequiresObservedConvergence(t *testing.T) {
	for _, converged := range []bool{true, false} {
		t.Run(map[bool]string{true: "removed", false: "residual-circuit"}[converged], func(t *testing.T) {
			p := tempPaths(t)
			old := []byte("frr version 10.7.1\nrouter isis vrx\n net 49.0001.0000.0000.0001.00\nexit\nend\n")
			if err := os.WriteFile(p.ConfFile(), old, 0600); err != nil {
				t.Fatal(err)
			}
			reloads := 0
			rr := renderers.NewRecordingRunner().On(ReloadBin, func(c renderers.Command) (renderers.Output, error) {
				if c.Args[0] == "--test" {
					if converged {
						return renderers.Output{Stdout: []byte("Lines To Delete\n===============\nLines To Add\n============\n")}, nil
					}
					return renderers.Output{Stdout: []byte("Lines To Delete\n===============\ninterface w6-l0\n ip router isis vrx\nexit\nLines To Add\n============\n")}, nil
				}
				reloads++
				if reloads == 1 {
					out := renderers.Output{ExitCode: 1, Stderr: []byte("redundant circuit removal")}
					return out, &renderers.ExitError{Command: c, Output: out}
				}
				return renderers.Output{}, nil
			})
			r := New(rr, WithPaths(p), WithSections(), WithInterfaceMapper(IdentityMapper))
			files, err := r.Render(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			err = r.Apply(context.Background(), files)
			if converged {
				if err != nil || reloads != 1 {
					t.Fatalf("converged removal: err=%v reloads=%d", err, reloads)
				}
				b, _ := os.ReadFile(p.ConfFile())
				if strings.Contains(string(b), "router isis") {
					t.Fatal("removed configuration restored")
				}
			} else {
				if !errors.Is(err, ErrDaemon) || reloads != 2 {
					t.Fatalf("residual removal: err=%v reloads=%d", err, reloads)
				}
				b, _ := os.ReadFile(p.ConfFile())
				if string(b) != string(old) {
					t.Fatal("failed removal not rolled back")
				}
			}
		})
	}
}
