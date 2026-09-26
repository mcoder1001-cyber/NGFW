module ngfw/test/topology/acl

go 1.26

require (
	go.fd.io/govpp v0.13.0
	ngfw/agent v0.0.0
)

require (
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/lunixbochs/struc v0.0.0-20200521075829-a4cb8d33dbbe // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	golang.org/x/sys v0.47.0 // indirect
)

replace ngfw/agent => ../../../apps/agent
