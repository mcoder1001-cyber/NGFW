// Package bond holds the DF-1 descriptors of the bond plugin family; see the package documentation
// in the descriptor files and docs/agent/descriptors/bond.md for the object ↔ VPP message table.
package bond

// bond_model.pb.go is generated from bond_model.proto with the protoc-gen-go pinned by go.mod; the output is
// committed. Regenerate with: go generate ./internal/descriptors/...
//go:generate protoc -I . --go_out=. --go_opt=paths=source_relative bond_model.proto
