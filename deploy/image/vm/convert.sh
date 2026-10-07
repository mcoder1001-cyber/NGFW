#!/usr/bin/env bash
# Explicit raw input only: no backing-chain discovery or implicit format probing.
set -euo pipefail
HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)
[[ $# == 4 ]] || { echo 'usage: convert.sh RAW OUTPUT_DIR PROFILE SIZE_GIB' >&2; exit 2; }
RAW=$(realpath -e -- "$1") OUT=$(realpath -e -- "$2") PROFILE=$3 SIZE=$4
[[ -f $RAW && ! -b $RAW && $RAW != "$OUT"/* ]] || { echo 'raw input must be a separate regular file' >&2; exit 2; }
[[ $SIZE =~ ^[0-9]+$ && $SIZE -ge 96 && $SIZE -le 4096 ]] || exit 2
[[ $(stat -c %s "$RAW") -eq $((SIZE * 1024 * 1024 * 1024)) ]] || { echo 'raw size mismatch' >&2; exit 2; }
[[ $PROFILE =~ ^(vm|aws|azure|gcp)$ ]] || exit 2
[[ ! -e $OUT/ngfw.qcow2 && ! -e $OUT/ngfw.ova && ! -e $OUT/ngfw.vhdx ]] || { echo 'refuse existing outputs' >&2; exit 2; }
qemu-img convert -f raw -O qcow2 -o compat=1.1 "$RAW" "$OUT/ngfw.qcow2"
qemu-img convert -f raw -O vmdk -o subformat=streamOptimized "$RAW" "$OUT/ngfw.vmdk"
qemu-img convert -f raw -O vhdx -o subformat=dynamic "$RAW" "$OUT/ngfw.vhdx"
python3 "$HERE/image.py" ovf --disk "$OUT/ngfw.vmdk" --output "$OUT/ngfw.ovf" --size "$SIZE"
(
  cd "$OUT"
  for f in ngfw.ovf ngfw.vmdk; do printf 'SHA256(%s)= %s\n' "$f" "$(sha256sum "$f" | cut -d' ' -f1)"; done > ngfw.mf
  tar --format=ustar --owner=0 --group=0 --numeric-owner --mtime=@0 -cf ngfw.ova ngfw.ovf ngfw.mf ngfw.vmdk
)
case $PROFILE in
  aws) qemu-img convert -f raw -O vpc -o subformat=fixed,force_size=on "$RAW" "$OUT/ngfw-aws.vhd" ;;
  azure) qemu-img convert -f raw -O vpc -o subformat=fixed,force_size=on "$RAW" "$OUT/ngfw-azure.vhd" ;;
  gcp)
    # disk.raw must be the sole archive member. Sparse tar keeps zero extents cheap.
    mkdir "$OUT/gcp-stage"
    cp --sparse=always --reflink=auto "$RAW" "$OUT/gcp-stage/disk.raw"
    tar --sparse --format=oldgnu --owner=0 --group=0 --numeric-owner --mtime=@0 -C "$OUT/gcp-stage" -cf - disk.raw | gzip -n > "$OUT/ngfw-gcp.tar.gz"
    rm -- "$OUT/gcp-stage/disk.raw"; rmdir "$OUT/gcp-stage"
    ;;
esac
for f in "$OUT"/*.qcow2 "$OUT"/*.vmdk "$OUT"/*.vhdx "$OUT"/*.vhd; do
  [[ -f $f ]] || continue
  format=; case $f in *.qcow2) format=qcow2 ;; *.vmdk) format=vmdk ;; *.vhdx) format=vhdx ;; *.vhd) format=vpc ;; esac
  qemu-img info -f "$format" --output=json "$f" > "$f.info.json"
  # VHD/VHDX do not implement check; return 63 is documented unsupported, not success.
  rc=0; qemu-img check -f "$format" "$f" > "$f.check.txt" 2>&1 || rc=$?
  [[ $rc == 0 || ( $rc == 63 && ( $format == vpc || $format == vhdx || $format == vmdk ) ) ]] || { cat "$f.check.txt" >&2; exit "$rc"; }
  if [[ $rc == 63 ]]; then printf '\nStructural check unsupported; byte-for-byte qemu-img compare follows.\n' >> "$f.check.txt"; fi
  qemu-img compare -f raw -F "$format" "$RAW" "$f" >> "$f.check.txt"
done
