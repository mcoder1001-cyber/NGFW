package df6

// The desired-state models of the DF-6 descriptors are generated from the .proto files next
// to each package with the protoc-gen-go pinned by go.mod (v1.36.12). Run `go generate` in
// this directory after editing a .proto file; the .pb.go files are committed. They are
// agent-internal until the corresponding P03 proto types exist (then KeyOf/Retrieve switch
// to those, the field sets stay).

//go:generate protoc -I .. --go_out=.. --go_opt=paths=source_relative ../gre/model.proto
//go:generate protoc -I .. --go_out=.. --go_opt=paths=source_relative ../ipip/model.proto
//go:generate protoc -I .. --go_out=.. --go_opt=paths=source_relative ../vxlan/model.proto
//go:generate protoc -I .. --go_out=.. --go_opt=paths=source_relative ../vxlan_gpe/model.proto
//go:generate protoc -I .. --go_out=.. --go_opt=paths=source_relative ../gtpu/model.proto
//go:generate protoc -I .. --go_out=.. --go_opt=paths=source_relative ../l2tp/model.proto
//go:generate protoc -I .. --go_out=.. --go_opt=paths=source_relative ../pppoe/model.proto
//go:generate protoc -I .. --go_out=.. --go_opt=paths=source_relative ../sr/model.proto
//go:generate protoc -I .. --go_out=.. --go_opt=paths=source_relative ../sr_mpls/model.proto
//go:generate protoc -I .. --go_out=.. --go_opt=paths=source_relative ../lisp/model.proto
