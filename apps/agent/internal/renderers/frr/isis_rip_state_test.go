package frr

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestOptionalTextReaderPreservesJSONAndOnDemand(t *testing.T) {
	const textKey = "ripFixtureStatus"
	const command ShowCommand = "show ip rip status"
	answers := standardAnswers()
	answers[command] = "valid status"
	runner := showRunner(t, answers)
	reader := StateReader{Key: textKey, Command: command, OnDemand: true, Parse: func(raw []byte) (json.RawMessage, error) {
		if string(raw) != "valid status" {
			return nil, errors.New("sensitive malformed text")
		}
		return json.RawMessage(`{"peers":[]}`), nil
	}}
	r := New(runner, WithPaths(TestPaths("w12")), WithStateReaders(reader))
	state, e := r.State(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if _, ok := state.Extra[textKey]; ok {
		t.Fatal("automatic optional read")
	}
	raw, e := r.ReadState(context.Background(), reader)
	if e != nil || string(raw) != `{"peers":[]}` {
		t.Fatal(string(raw), e)
	}
	raw, e = r.ReadState(context.Background(), StateReader{Key: "fixtureJSON", Command: ShowInterfaceAll})
	if e != nil || string(raw) != answers[ShowInterfaceAll] {
		t.Fatal("JSON regression", e)
	}
	bad := reader
	bad.Parse = func([]byte) (json.RawMessage, error) { return nil, errors.New("sensitive malformed text") }
	if _, e = r.ReadState(context.Background(), bad); e == nil || strings.Contains(e.Error(), "sensitive") {
		t.Fatal("unsanitized parser error", e)
	}
	bad.Parse = func([]byte) (json.RawMessage, error) { return json.RawMessage("invalid"), nil }
	if _, e = r.ReadState(context.Background(), bad); e == nil {
		t.Fatal("invalid projected JSON")
	}
}
