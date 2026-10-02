package basepolicy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"ngfw/agent/internal/renderers"
	"slices"
	"strings"
	"testing"
)

func kernel(names []string) []byte {
	b, _ := json.Marshal(map[string]any{"nftables": []any{map[string]any{"set": map[string]any{"family": "inet", "table": "vrx_base", "name": "punt_interfaces", "type": "ifname", "elem": names}}}})
	return b
}
func TestMembers(t *testing.T) {
	got, err := Members("mgmt0", []string{"tap2"}, []string{"tap1", "tap2"})
	if err != nil || !slices.Equal(got, []string{"tap1", "tap2"}) {
		t.Fatalf("union %v %v", got, err)
	}
	for _, names := range [][]string{{"lo"}, {"mgmt0"}, {"tap;accept"}, {"tap0", "tap0"}, {strings.Repeat("x", 16)}} {
		if _, err := Members("mgmt0", nil, names); err == nil {
			t.Fatalf("accepted %v", names)
		}
	}
	names := make([]string, 64)
	for i := range names {
		names[i] = fmt.Sprintf("tap%d", i)
	}
	if _, err := Members("mgmt0", nil, names); err != nil {
		t.Fatal(err)
	}
	if _, err := Members("mgmt0", []string{"extra"}, names); err == nil {
		t.Fatal("65-member union accepted")
	}
}
func TestParse(t *testing.T) {
	if _, err := Parse(kernel(nil), "mgmt0"); err != nil {
		t.Fatal(err)
	}
	bad := [][]byte{[]byte(`{}`), []byte(`{"nftables":[{"table":{}}]}`), []byte(strings.Repeat("x", MaxJSON+1)), append(kernel(nil), []byte(` {}`)...), kernel([]string{"mgmt0"}), kernel([]string{"tap0", "tap0"})}
	for _, replacement := range []string{"wrong", "inet vrx"} {
		bad = append(bad, []byte(strings.Replace(string(kernel(nil)), "vrx_base", replacement, 1)))
	}
	bad = append(bad, []byte(strings.Replace(string(kernel(nil)), `"ifname"`, `"ipv4_addr"`, 1)), []byte(strings.Replace(string(kernel(nil)), `"type":"ifname"`, `"type":"ifname","flags":["interval"]`, 1)))
	for _, data := range bad {
		if _, err := Parse(data, "mgmt0"); err == nil {
			t.Fatalf("accepted %s", data)
		}
	}
}

type fakeNft struct {
	state []string
	calls []renderers.Command
	fail  int
}

func (f *fakeNft) Run(_ context.Context, c renderers.Command) (renderers.Output, error) {
	f.calls = append(f.calls, c)
	if c.Path != NftBin {
		return renderers.Output{}, errors.New("wrong executable")
	}
	if len(f.calls) == f.fail {
		return renderers.Output{}, errors.New("injected nft failure")
	}
	if slices.Equal(c.Args, []string{"-j", "list", "set", "inet", "vrx_base", "punt_interfaces"}) {
		return renderers.Output{Stdout: kernel(f.state)}, nil
	}
	if !slices.Equal(c.Args, []string{"-c", "-f", "-"}) && !slices.Equal(c.Args, []string{"-f", "-"}) {
		return renderers.Output{}, errors.New("unexpected argv")
	}
	if !strings.HasPrefix(string(c.Stdin), "flush set inet vrx_base punt_interfaces\n") {
		return renderers.Output{}, errors.New("foreign mutation")
	}
	if slices.Equal(c.Args, []string{"-f", "-"}) {
		f.state = nil
		for _, token := range strings.Split(string(c.Stdin), `"`)[1:] {
			if validName(token) {
				f.state = append(f.state, token)
			}
		}
	}
	return renderers.Output{}, nil
}
func TestAtomicReplacementFailureAndNoOp(t *testing.T) {
	for _, fail := range []int{1, 2, 3} {
		f := &fakeNft{state: []string{"old"}, fail: fail}
		r, _ := New(f, "mgmt0")
		if err := r.Replace(context.Background(), []string{"new"}); err == nil {
			t.Fatal("expected failure")
		}
		if !slices.Equal(f.state, []string{"old"}) {
			t.Fatal("failed transaction mutated set")
		}
	}
	f := &fakeNft{state: []string{"old"}}
	r, _ := New(f, "mgmt0")
	if err := r.Replace(context.Background(), []string{"old"}); err != nil || len(f.calls) != 1 {
		t.Fatal("no-op wrote")
	}
	if err := r.Replace(context.Background(), []string{"new"}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(f.state, []string{"new"}) {
		t.Fatal(f.state)
	}
	if err := r.Replace(context.Background(), nil); err != nil || len(f.state) != 0 {
		t.Fatal("empty replacement failed")
	}
}
