package keepalived

import (
	"testing"

	"google.golang.org/protobuf/types/known/structpb"
)

func doc(t testing.TB, m map[string]any) *structpb.Struct {
	t.Helper()
	s, err := structpb.NewStruct(m)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
