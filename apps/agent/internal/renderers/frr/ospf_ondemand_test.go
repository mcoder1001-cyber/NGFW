package frr

import (
	"context"
	"testing"
)

func TestOnDemandReaderSkipsAutomaticStateButStillReadsExplicitly(t *testing.T) {
	const observationName = "ospf6Neighbors"
	answers := standardAnswers()
	const command ShowCommand = "show ipv6 ospf6 neighbor json"
	answers[command] = `{"neighbors":[]}`
	runner := showRunner(t, answers)
	r := New(runner, WithPaths(TestPaths("w12")), WithStateReaders(StateReader{Key: observationName, Command: command, OnDemand: true}))
	state, err := r.State(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, present := state.Extra[observationName]; present {
		t.Fatal("optional reader automatically loaded")
	}
	for _, call := range runner.Calls() {
		if ShowCommand(call.Args[len(call.Args)-1]) == command {
			t.Fatal("automatic read ran optional command")
		}
	}
	observed, err := r.ShowJSON(context.Background(), command)
	if err != nil || string(observed) != answers[command] {
		t.Fatalf("explicit reader: %s %v", observed, err)
	}
}
