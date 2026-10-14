#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

GUM="gum"
command -v gum &>/dev/null || GUM="nix run nixpkgs#gum --"

# ── 1. Select USB drive first (before any download) ──────────────────────────

mapfile -t USB_DISKS < <(
  lsblk -dpno NAME,TRAN,SIZE | awk '$2=="usb" && $3!="0B"{print $1}' | while read -r dev; do
    size=$(lsblk -dno SIZE "$dev")
    model=$(lsblk -dno MODEL "$dev" | xargs)
    parts=$(lsblk -no FSTYPE,LABEL "$dev" 2>/dev/null \
      | awk 'NF { gsub(/^[[:space:]]+|[[:space:]]+$/, ""); if (!seen[$0]++) buf = buf (buf?", ":"") $0 }
             END { print buf ? "["buf"]" : "[empty]" }')
    parts="${parts:0:40}"
    printf "%s  %s  %s  %s\n" "$dev" "$size" "$model" "$parts"
  done
)

if [[ "${#USB_DISKS[@]}" -eq 0 ]]; then
  echo "ERROR: no USB drives found. Plug in a flash drive and retry." >&2; exit 1
fi

TARGET_CHOICE="$($GUM choose --header "Select USB drive to flash (ALL DATA WILL BE LOST):" "${USB_DISKS[@]}")"
TARGET="$(echo "$TARGET_CHOICE" | awk '{print $1}')"

echo ""
echo "Current partition layout of ${TARGET}:"
echo "─────────────────────────────────────────────"
lsblk -o NAME,SIZE,FSTYPE,LABEL,MOUNTPOINT "$TARGET"
echo "─────────────────────────────────────────────"
echo ""

TYPED="$($GUM input --placeholder "Type \"${TARGET}\" to confirm erasure")"
if [[ "$TYPED" != "$TARGET" ]]; then
  echo "Aborted: input did not match." >&2; exit 1
fi

# ── 2. Select ISO source ──────────────────────────────────────────────────────

echo ""
SOURCE="$($GUM choose --header "ISO source:" \
  "Local file" \
  "Download NixOS")"

case "$SOURCE" in
  "Local file")
    echo ""
    echo "Select ISO file:"
    ISO="$($GUM file --file --height=20 "${1:-$HOME}")"
    if [[ ! -f "$ISO" ]]; then
      echo "ERROR: file not found: ${ISO}" >&2; exit 1
    fi
    ;;

  "Download NixOS")
    CHANNEL="$(grep -oP 'nixpkgs/nixos-\K[\d.]+' "${SCRIPT_DIR}/flake.nix" | head -1 || true)"
    CHANNEL="${CHANNEL:-25.11}"

    VARIANT="$($GUM choose --header "NixOS variant:" \
      "minimal  – CLI only, small (~900MB)" \
      "gnome    – graphical installer (~2.5GB)")"
    VARIANT="$(echo "$VARIANT" | awk '{print $1}')"

    URL="https://channels.nixos.org/nixos-${CHANNEL}/latest-nixos-${VARIANT}-x86_64-linux.iso"
    ISO_NAME="nixos-${CHANNEL}-${VARIANT}-x86_64.iso"
    ISO="/tmp/${ISO_NAME}"

    echo ""
    echo "==> Downloading ${ISO_NAME} ..."
    echo "    from: ${URL}"
    echo ""
    curl -L --progress-bar -o "$ISO" "$URL"

    ;;
esac

# ── 3. Flash ──────────────────────────────────────────────────────────────────

ISO_SIZE="$(du -sh "$ISO" | awk '{print $1}')"
echo ""
echo "==> Flashing $(basename "$ISO") (${ISO_SIZE}) → ${TARGET} ..."

while read -r part; do
  mountpoint -q "$part" 2>/dev/null && sudo umount "$part" || true
done < <(lsblk -lno NAME "$TARGET" | tail -n +2 | awk '{print "/dev/"$1}')

sudo dd if="$ISO" of="$TARGET" bs=4M status=progress oflag=direct

echo ""
echo "==> Done. You can remove the flash drive."
