# Independent images T3 fixtures

Source 6bd58ecbe10efb45a486139777655c6c826093cd; archive /dev/shm/r4i; 2026-10-05 UTC. No slot/runtime appliance.

Command: `cd /dev/shm/r4i && python3 -m unittest discover -s test/topology/images -v`

```text
test_complete_inspection_and_secret_rejection (test_images.ImageTests.test_complete_inspection_and_secret_rejection) ... ok
test_completion_bridge_tracks_current_packaging (test_images.ImageTests.test_completion_bridge_tracks_current_packaging) ... ok
test_every_mutation_parent_preflight_preserves_outside (test_images.ImageTests.test_every_mutation_parent_preflight_preserves_outside) ... ok
test_grub_preflight_preserves_external_boot_and_config (test_images.ImageTests.test_grub_preflight_preserves_external_boot_and_config) ... ok
test_grub_rejects_host_root_and_aliased_roots_before_kernel_read (test_images.ImageTests.test_grub_rejects_host_root_and_aliased_roots_before_kernel_read) ... ok
test_grub_requires_kernel_initrd_and_shared_cmdline (test_images.ImageTests.test_grub_requires_kernel_initrd_and_shared_cmdline) ... ok
test_layout_consumes_shared_labels (test_images.ImageTests.test_layout_consumes_shared_labels) ... ok
test_ovf_references_hardware_and_tar_safe_filename (test_images.ImageTests.test_ovf_references_hardware_and_tar_safe_filename) ... ok
test_profiles_and_identity (test_images.ImageTests.test_profiles_and_identity) ... ok
test_real_small_format_roundtrips (test_images.ImageTests.test_real_small_format_roundtrips) ... ok
test_required_files_packages_password_and_datasource (test_images.ImageTests.test_required_files_packages_password_and_datasource) ... ok
test_root_build_is_opt_in_and_help_is_safe (test_images.ImageTests.test_root_build_is_opt_in_and_help_is_safe) ... ok
test_target_cannot_escape_via_symlink (test_images.ImageTests.test_target_cannot_escape_via_symlink) ... ok

----------------------------------------------------------------------
Ran 13 tests in 0.738s

OK
```

Command: `python3 /dev/shm/r4t/adversarial.py`

```text
host-root: refused
root-alias: refused
ancestor-alias: refused
outside boot parent: refused, sentinel unchanged
output symlink: refused, sentinel unchanged
5 adversarial checks PASS; all writes confined to owned RAM fixture
```

Adversarial script preserved in R4-closures-evidence/adversarial.py. PASS covers offline fixture writes and format roundtrips only. VM/cloud boot, real image installation and appliance traffic NOT RUN. No host boot files or services modified.

Verdict: PASS (actual offline fixture scope).

| Scenario | Expected | Observed | Result |
|---|---|---|---|
| Offline13 cases | All pass | 13/13 | PASS |
| Five adversarial paths | Refusal, outside unchanged | 5/5 | PASS |
| Actual VM/cloud boot | Running appliance | Unprovisioned, not run | NOT RUN |
