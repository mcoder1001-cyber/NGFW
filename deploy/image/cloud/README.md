# Cloud overlays

`profiles.json` is consumed by `../vm/image.py`; it is the sole cloud datasource
and early-driver map. Build a separate raw appliance root for each profile, then
convert it. AWS and Azure use fixed VHD; Azure virtual size is aligned to 1 MiB.
GCP uses a sparse `disk.raw` gzip tar. No credentials or provider writes are part
of these files. Import examples and acceptance plan: `docs/install/images.md`.

P14/P10 kernel and firstboot defaults remain shared. ENA/Hyper-V/virtio modules do
not bind DPDK devices; cloud images retain `dpdk { no-pci }` until the wizard.
