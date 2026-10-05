# Appliance disk builder

Run `build.sh --help`. Required signed inputs and guest management NIC are explicit;
root disk creation requires `NGFW_INTEGRATION=1`, an isolated worktree and 40 GiB
free. Mounts are private and GRUB runs inside the image, never against host /boot.
`build-root.sh` is an internal namespace-only implementation, not an entrypoint.
P14's `../iso/common/` files and VPP verifier are reused read-only.

Host-independent plan/configuration/inspection tests:

```
python3 -m unittest discover -s test/topology/images -p 'test_*.py' -v
shellcheck -x deploy/image/vm/*.sh
(cd test/topology/images && go test -count=1 ./...)
```

The unchanged quick gate discovers the Go test module and executes Python source
checks plus real 16 MiB format roundtrips when qemu-img is available. They never
boot or create loop devices. Integration inspection needs `NGFW_INTEGRATION=1`
and an explicit `NGFW_IMAGE_ROOT`; these fixtures are not appliance acceptance.
