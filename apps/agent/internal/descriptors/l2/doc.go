// Package l2 holds the DF-1 descriptors of the l2 plugin family; see the package documentation
// in the descriptor files and docs/agent/descriptors/l2.md for the object ↔ VPP message table.
package l2

// l2_model.pb.go is generated from l2_model.proto with the protoc-gen-go pinned by go.mod; the output is
// committed. Regenerate with: go generate ./internal/descriptors/...
//go:generate protoc -I . --go_out=. --go_opt=paths=source_relative l2_model.proto
