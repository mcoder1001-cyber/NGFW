module ngfw/test/topology/nat44-ei-64-66-nptv6

go 1.26

require (
	go.fd.io/govpp v0.13.0
	golang.org/x/net v0.57.0
	google.golang.org/grpc v1.84.0
	google.golang.org/protobuf v1.36.12
	ngfw/agent v0.0.0
)

require (
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/lunixbochs/struc v0.0.0-20200521075829-a4cb8d33dbbe // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.40.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260706201446-f0a921348800 // indirect
)

replace ngfw/agent => ../../../apps/agent
