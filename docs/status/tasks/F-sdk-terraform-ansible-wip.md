# F-sdk-terraform-ansible — WIP

- [x] live env script `test/topology/sdk-terraform-ansible/live.sh` (slot 5: pg vrx_w5, real vrx-agent owner w5, API :3500, API key in /run)
- [x] Python SDK `sdk/python` (stdlib generator `tools/gen.py` → `vrx/_generated`, `VrxSession`, typed errors, redaction), 27 unit tests, live test green
- [ ] Terraform provider `sdk/terraform` (vrx_config, vrx_interface generated, data vrx_state) + protocol-level harness (no terraform CLI on the host)
- [ ] `sdk/gen.sh` (+ `--check`)
- [ ] docs/user/system/sdk-terraform-ansible.md
- [ ] Ansible — cut (D-085), listed as left over
- [ ] CI gate, status file, env down, vrx_w5 dropped
