# Private peer asset visibility

The disposable peer launcher now mirrors the reviewed production daemon individual asset view: root-private readonly instance tmpfs, only network.json/netns/strongswan.conf/swanctl.conf and private/x509/x509ca/x509crl directories, writable daemon child. It refuses hostnetns visibility. The temporary held source remains outside the masked /run and is hidden with the host /dev/shm before dropping capability; inherited host MNT fd3 still closes before exec. No production authorization or capability changes.

Python AST syntax passes. No actual mount/helper/client acceptance run; independent review and full guest required. Canonical test drivers and immutable v3/Hold test ELFs unchanged; the guest recipe must stage this exact companion script and new reviewed production helper.
