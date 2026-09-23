package rsyslog

import (
	"testing"

	"google.golang.org/protobuf/types/known/structpb"
)

func doc(t testing.TB, targets []any) *structpb.Struct {
	t.Helper()
	s, err := structpb.NewStruct(map[string]any{"management": map[string]any{"syslog": targets}})
	if err != nil {
		t.Fatal(err)
	}
	return s
}
