// Package basepolicy owns only the appliance's typed punt interface set.
package basepolicy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
	"time"

	"ngfw/agent/internal/renderers"
)

const NftBin = "/usr/sbin/nft"
const MaxMembers = 64
const MaxJSON = 32768

var interfaceName = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,14}$`)

// Members validates the permanent/dynamic union before any command. Duplicate
// names within a source are rejected; overlap between sources is a set union.
func Members(management string, permanent, dynamic []string) ([]string, error) {
	if !validName(management) {
		return nil, errors.New("basepolicy: invalid management interface")
	}
	union := map[string]bool{}
	for _, source := range [][]string{permanent, dynamic} {
		seen := map[string]bool{}
		if len(source) > MaxMembers {
			return nil, errors.New("basepolicy: too many interfaces")
		}
		for _, name := range source {
			if !validName(name) || name == management || seen[name] {
				return nil, errors.New("basepolicy: invalid or duplicate punt interface")
			}
			seen[name] = true
			union[name] = true
		}
	}
	if len(union) > MaxMembers {
		return nil, errors.New("basepolicy: punt union exceeds 64")
	}
	out := make([]string, 0, len(union))
	for name := range union {
		out = append(out, name)
	}
	slices.Sort(out)
	return out, nil
}
func validName(name string) bool { return name != "lo" && interfaceName.MatchString(name) }

// Transaction is an atomic replacement of one set, never a table/ruleset flush.
func Transaction(management string, members []string) ([]byte, error) {
	names, err := Members(management, nil, members)
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	b.WriteString("flush set inet vrx_base punt_interfaces\n")
	if len(names) > 0 {
		quoted := make([]string, len(names))
		for i, name := range names {
			data, _ := json.Marshal(name)
			quoted[i] = string(data)
		}
		fmt.Fprintf(&b, "add element inet vrx_base punt_interfaces { %s }\n", strings.Join(quoted, ", "))
	}
	return []byte(b.String()), nil
}

// Parse reads only the exact simple ifname set; extensions with flags,
// expressions, timeouts or unknown top-level objects fail closed.
func Parse(data []byte, management string) ([]string, error) {
	if len(data) > MaxJSON {
		return nil, errors.New("basepolicy: oversized nft JSON")
	}
	var document struct {
		Nftables []map[string]json.RawMessage `json:"nftables"`
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&document); err != nil {
		return nil, fmt.Errorf("basepolicy: nft JSON: %w", err)
	}
	if dec.Decode(new(any)) != io.EOF {
		return nil, errors.New("basepolicy: trailing nft JSON")
	}
	var result []string
	found := false
	for _, item := range document.Nftables {
		if len(item) != 1 {
			return nil, errors.New("basepolicy: malformed nft object")
		}
		if _, ok := item["metainfo"]; ok {
			continue
		}
		raw, ok := item["set"]
		if !ok || found {
			return nil, errors.New("basepolicy: unexpected nft object")
		}
		var set struct {
			Family string   `json:"family"`
			Table  string   `json:"table"`
			Name   string   `json:"name"`
			Type   string   `json:"type"`
			Handle uint64   `json:"handle"`
			Elem   []string `json:"elem"`
		}
		sd := json.NewDecoder(bytes.NewReader(raw))
		sd.DisallowUnknownFields()
		if err := sd.Decode(&set); err != nil {
			return nil, fmt.Errorf("basepolicy: invalid set: %w", err)
		}
		if set.Family != "inet" || set.Table != "vrx_base" || set.Name != "punt_interfaces" || set.Type != "ifname" {
			return nil, errors.New("basepolicy: wrong set identity/type")
		}
		var err error
		result, err = Members(management, nil, set.Elem)
		if err != nil {
			return nil, err
		}
		found = true
	}
	if !found {
		return nil, errors.New("basepolicy: missing punt set")
	}
	return result, nil
}

// Renderer uses the root namespace runner only. Namespace selection belongs to
// the typed projection and is never expressed through arbitrary command args.
type Renderer struct {
	runner     renderers.Runner
	management string
}

func New(runner renderers.Runner, management string) (*Renderer, error) {
	if _, err := Members(management, nil, nil); err != nil {
		return nil, err
	}
	if runner == nil {
		return nil, errors.New("basepolicy: missing nft runner")
	}
	return &Renderer{runner: runner, management: management}, nil
}
func (r *Renderer) Retrieve(ctx context.Context) ([]string, error) {
	out, err := r.runner.Run(ctx, renderers.Command{Path: NftBin, Args: []string{"-j", "list", "set", "inet", "vrx_base", "punt_interfaces"}, Timeout: 10 * time.Second})
	if err != nil {
		return nil, err
	}
	if out.ExitCode != 0 {
		return nil, errors.New("basepolicy: nft read failed")
	}
	return Parse(out.Stdout, r.management)
}

// Replace validates against the current kernel set, then atomically replaces
// membership. A rejected nft transaction leaves previous membership intact.
// Transaction/descriptor ownership must journal compensation after success.
func (r *Renderer) Replace(ctx context.Context, members []string) error {
	input, err := Transaction(r.management, members)
	if err != nil {
		return err
	}
	desired, _ := Members(r.management, nil, members)
	actual, err := r.Retrieve(ctx)
	if err != nil {
		return err
	}
	if slices.Equal(actual, desired) {
		return nil
	}
	for _, args := range [][]string{{"-c", "-f", "-"}, {"-f", "-"}} {
		out, err := r.runner.Run(ctx, renderers.Command{Path: NftBin, Args: args, Stdin: input, Timeout: 10 * time.Second})
		if err != nil {
			return err
		}
		if out.ExitCode != 0 {
			return errors.New("basepolicy: nft transaction rejected")
		}
	}
	return nil
}
