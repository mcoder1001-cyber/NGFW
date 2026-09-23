// Package renderers defines the contract for the GPL daemon renderers (FRR, strongSwan, Kea,
// Unbound, chrony, keepalived, snmpd, rsyslog — tasks P11, P12, RF-*) and the helpers they
// share: atomic file writes with snapshot/restore, a fixed-argv allow-listed process runner
// and strict template escaping. The daemons stay separate processes driven by rendered
// config files and their own control channels (00-CONTEXT rules 7 and 9); no user input ever
// reaches a shell.
//
// # Lifecycle of one renderer inside a commit (docs/01-architecture.md AD-4)
//
//	Render(desired)  → Files      pure: no I/O, deterministic, golden-testable
//	Validate(files)  → error      the daemon's own dry-run against a Staging copy; never the live paths
//	Apply(files)     → error      TakeSnapshot → WriteFiles (atomic) → reload through the daemon's
//	                              control channel → on failure Restore the snapshot, reload again, return the error
//	Retrieve()       → proto      actual daemon state as structured data (its JSON/show output)
//
// The commit engine calls Render and Validate for every renderer before it applies anything
// (tier-3 validation), then Apply in backend order with the risky backends last, and on any
// failure Apply(previousFiles) in reverse order. Apply must therefore be idempotent and safe
// to call with the previous rendering. Secrets (Files with Secret set) are never logged,
// never included in diagnostics and written with a mode that is not world-readable.
package renderers

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"google.golang.org/protobuf/proto"
)

// Renderer turns desired state for one daemon into config files, validates and applies them,
// and reads the daemon's actual state back. One renderer per daemon; see README.md.
type Renderer interface {
	// Name is the daemon name in lower-case: "frr", "strongswan", "kea-dhcp4", "unbound", ….
	Name() string
	// Render produces the complete set of files for desired. It performs no I/O and is
	// deterministic; every user string passes through the strict escaping helpers.
	Render(ctx context.Context, desired proto.Message) (Files, error)
	// Validate runs the daemon's dry-run (vtysh -C -f, kea-dhcp4 -t, unbound-checkconf, …) on a
	// staged copy of files. It must not modify the live configuration. Renderers of daemons
	// without a checker validate structurally and document that in their README.
	Validate(ctx context.Context, files Files) error
	// Apply writes files atomically and reloads the daemon through its control channel. On any
	// failure it restores the previous files and returns the error.
	Apply(ctx context.Context, files Files) error
	// Retrieve returns the daemon's actual state decoded into a proto message (the desired
	// type where the daemon can report it, otherwise the domain's state message).
	Retrieve(ctx context.Context) (proto.Message, error)
}

// File is one rendered configuration file.
type File struct {
	// Mode is the permission bits (0o644, 0o600, …). Zero is rejected: state the mode.
	Mode os.FileMode
	// Owner is "user" or "user:group" by name; "" keeps the writing process's identity.
	Owner string
	// Content is the complete file content.
	Content []byte
	// Secret marks files holding PSKs, keys or passwords: never logged or shown, and the mode
	// must not be world-readable.
	Secret bool
}

// Files maps absolute, clean target paths to their File.
type Files map[string]File

// ErrInvalidFiles is wrapped by every Validate error.
var ErrInvalidFiles = errors.New("renderers: invalid files")

// Validate checks the map: paths are absolute and clean (no ".", "..", trailing slash),
// modes are non-zero permission bits only, and Secret files are not world-readable.
func (f Files) Validate() error {
	for p, file := range f {
		switch {
		case !filepath.IsAbs(p):
			return fmt.Errorf("%w: path %q is not absolute", ErrInvalidFiles, p)
		case filepath.Clean(p) != p:
			return fmt.Errorf("%w: path %q is not clean (want %q)", ErrInvalidFiles, p, filepath.Clean(p))
		case p == string(filepath.Separator):
			return fmt.Errorf("%w: path %q is the root", ErrInvalidFiles, p)
		case file.Mode == 0:
			return fmt.Errorf("%w: %s has no mode", ErrInvalidFiles, p)
		case file.Mode&^os.ModePerm != 0:
			return fmt.Errorf("%w: %s mode %v has non-permission bits", ErrInvalidFiles, p, file.Mode)
		case file.Secret && file.Mode&0o007 != 0:
			return fmt.Errorf("%w: secret file %s is world-readable (%v)", ErrInvalidFiles, p, file.Mode)
		}
	}
	return nil
}

// Paths returns the target paths in sorted order (the order WriteFiles uses).
func (f Files) Paths() []string {
	out := make([]string, 0, len(f))
	for p := range f {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// Redacted returns a copy in which the Content of every Secret file is replaced by
// "<redacted>", for logs, diffs and dry-run output.
func (f Files) Redacted() Files {
	out := make(Files, len(f))
	for p, file := range f {
		if file.Secret {
			file.Content = []byte("<redacted>")
		}
		out[p] = file
	}
	return out
}
