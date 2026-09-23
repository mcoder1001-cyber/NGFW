// Package l3xc holds the DF-1 descriptors of the l3xc plugin family; see the package documentation
// in the descriptor files and docs/agent/descriptors/l3xc.md for the object ↔ VPP message table.
package l3xc

// l3xc_model.pb.go is generated from l3xc_model.proto with the protoc-gen-go pinned by go.mod; the output is
// committed. Regenerate with: go generate ./internal/descriptors/...
//go:generate protoc -I . --go_out=. --go_opt=paths=source_relative l3xc_model.proto
