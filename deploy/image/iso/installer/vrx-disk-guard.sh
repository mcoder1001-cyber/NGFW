# shellcheck shell=bash
# Read-only inventory guard shared by the early installer and offline regression tests.
# Never turn a failed or partial lsblk result into permission to erase a disk.
vrx_check_reinstall() {
  local cmdline=$1 labels partition filesystem
  if ! labels=$(lsblk -rno PARTLABEL,LABEL 2>/dev/null); then
    printf '%s\n' 'VRX disk inventory failed: cannot determine whether an installation already exists' >&2
    return 1
  fi
  if [[ " $cmdline " == *" vrx.reinstall=1 "* ]]; then
    return 0
  fi
  while read -r partition filesystem; do
    if [[ $partition == vrx-rootA || $filesystem == vrx-rootA ]]; then
      printf '%s\n' "a VRX installation already exists: remove the installation medium or explicitly choose Reinstall VRX to erase it" >&2
      return 1
    fi
  done <<< "$labels"
  return 0
}
