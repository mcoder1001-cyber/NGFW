package df2

// The desired-state models of the DF-2 descriptors are generated from the .proto files next
// to each package with the protoc-gen-go pinned by go.mod (v1.36.12). Run `go generate` in
// this directory after editing a .proto file; the .pb.go files are committed.

//go:generate protoc -I .. --go_out=.. --go_opt=paths=source_relative ../df2/df2.proto
//go:generate protoc -I .. --go_out=.. --go_opt=paths=source_relative ../ip_neighbor/model.proto
//go:generate protoc -I .. --go_out=.. --go_opt=paths=source_relative ../arp/model.proto
//go:generate protoc -I .. --go_out=.. --go_opt=paths=source_relative ../ip6_nd/model.proto
//go:generate protoc -I .. --go_out=.. --go_opt=paths=source_relative ../urpf/model.proto
//go:generate protoc -I .. --go_out=.. --go_opt=paths=source_relative ../adl/model.proto
//go:generate protoc -I .. --go_out=.. --go_opt=paths=source_relative ../abf/model.proto
//go:generate protoc -I .. --go_out=.. --go_opt=paths=source_relative ../classify/model.proto
//go:generate protoc -I .. --go_out=.. --go_opt=paths=source_relative ../ip_session_redirect/model.proto
