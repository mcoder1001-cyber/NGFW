// Package iface holds the DF-1 descriptors of the interface plugin family; see the package documentation
// in the descriptor files and docs/agent/descriptors/interface.md for the object ↔ VPP message table.
package iface

// iface_model.pb.go is generated from iface_model.proto with the protoc-gen-go pinned by go.mod; the output is
// committed. Regenerate with: go generate ./internal/descriptors/...
//go:generate protoc -I . --go_out=. --go_opt=paths=source_relative iface_model.proto
