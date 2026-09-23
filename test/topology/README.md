# Lab inventory (`tools/lab`)

One YAML file per machine. `tools/lab <cmd> <name>` loads `test/topology/<name>.yml`.

| key | meaning |
|---|---|
| `name` | machine name (= file name) |
| `role` | `vrx` (runs VPP + our stack), `host` (traffic endpoint), `peer-frr`, `peer-sswan` |
| `mgmt` | `local` → this host, commands run directly (no SSH); otherwise the management IP for `ssh root@<ip>` |
| `state` | `active` (exists, reachable) or `planned` (shape only — see `docs/lab/vmware.md`) |
| `os`, `vcpu`, `ram_gb` | VM shape |
| `vpp.*` | version, source of our `.deb`s (D-001), socket paths, `startup_conf`, `handover_doc` |
| `data_path` | `af_packet` (veth/netns rig, D-010) or `dpdk` (vmxnet3 bound to DPDK) |
| `nics[]` | `name`, `pci`, `model`, `segment` (`mgmt`/`lan`/`wan`/`p2p`/`unassigned`), `driver` (`kernel`/`dpdk`), `address` |
| `services.*` | control-plane services `tools/lab status` checks (PostgreSQL, Valkey) |

`vrx-a.yml` is the dev/CI host itself and is the only `active` entry today. Facts come from `docs/lab/host-vrx-a.md`;
re-verify anything marked volatile there before relying on it. The planned files are the shapes the product owner will
create later (`docs/lab/vmware.md`); `tools/lab provision <planned-vm>` prints the plan and refuses to run.
