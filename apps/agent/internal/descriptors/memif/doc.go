// Package memif holds the DF-1 descriptors of the memif plugin family; see the package documentation
// in the descriptor files and docs/agent/descriptors/memif.md for the object ↔ VPP message table.
package memif

// memif_model.pb.go is generated from memif_model.proto with the protoc-gen-go pinned by go.mod; the output is
// committed. Regenerate with: go generate ./internal/descriptors/...
//go:generate protoc -I . --go_out=. --go_opt=paths=source_relative memif_model.proto
