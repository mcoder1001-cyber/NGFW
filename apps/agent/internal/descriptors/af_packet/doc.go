// Package afpacket holds the DF-1 descriptors of the af_packet plugin family; see the package documentation
// in the descriptor files and docs/agent/descriptors/af_packet.md for the object ↔ VPP message table.
package afpacket

// afpacket_model.pb.go is generated from afpacket_model.proto with the protoc-gen-go pinned by go.mod; the output is
// committed. Regenerate with: go generate ./internal/descriptors/...
//go:generate protoc -I . --go_out=. --go_opt=paths=source_relative afpacket_model.proto
