#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Interactive helpers — gum if available, plain bash select otherwise
_choose() {
  local header="$1"; shift
  if command -v gum &>/dev/null; then
    gum choose --header "$header" "$@"
  else
    echo "$header" >&2
    select item in "$@"; do
      [[ -n "$item" ]] && echo "$item" && return
    done
  fi
}

_input() {
  local placeholder="$1"
  if command -v gum &>/dev/null; then
    gum input --placeholder "$placeholder"
  else
    read -rp "$placeholder: " val && echo "$val"
  fi
}

# Collect hosts dynamically (exclude 'default' — it's a shared base, not a real host)
mapfile -t HOSTS < <(
  find "${SCRIPT_DIR}/hosts" -mindepth 1 -maxdepth 1 -type d \
    ! -name default -printf '%f\n' | sort
)

if [[ "${#HOSTS[@]}" -eq 0 ]]; then
  echo "ERROR: no hosts found in ${SCRIPT_DIR}/hosts/"
  exit 1
fi

HOST="$(_choose "Select host to install:" "${HOSTS[@]}")"

# Build disk list with partition info so the user can see what's on each disk
mapfile -t DISKS < <(
  lsblk -dpno NAME | grep -v "loop\|sr" | while read -r dev; do
    size=$(lsblk -dno SIZE "$dev")
    model=$(lsblk -dno MODEL "$dev" | xargs)
    parts=$(lsblk -no FSTYPE,LABEL,MOUNTPOINT "$dev" 2>/dev/null \
      | awk 'NF { line=$0; gsub(/^[[:space:]]+|[[:space:]]+$/, "", line); buf = buf (buf?", ":"") line }
             END { print buf ? "["buf"]" : "[empty]" }')
    printf "%s  %s  %s  %s\n" "$dev" "$size" "$model" "$parts"
  done
)
if [[ "${#DISKS[@]}" -eq 0 ]]; then
  echo "ERROR: no block devices found" >&2
  exit 1
fi

DISK_CHOICE="$(_choose "Select disk to install on (ALL DATA WILL BE LOST):" "${DISKS[@]}")"
DISK="$(echo "$DISK_CHOICE" | awk '{print $1}')"

# Show full partition layout of the chosen disk
echo ""
echo "Current partition layout of ${DISK}:"
echo "─────────────────────────────────────────────"
lsblk -o NAME,SIZE,FSTYPE,LABEL,MOUNTPOINT "$DISK"
echo "─────────────────────────────────────────────"
echo ""

# Require typing the disk path manually to confirm
TYPED="$(_input "Type \"${DISK}\" to confirm erasure")"
if [[ "$TYPED" != "$DISK" ]]; then
  echo "Aborted: input did not match." >&2
  exit 1
fi

# Patch device path into a temp disko config
TMP_DISKO="$(mktemp --suffix=.nix)"
trap 'rm -f "$TMP_DISKO"' EXIT
sed "s|device = \"[^\"]*\";|device = \"${DISK}\";|" \
  "${SCRIPT_DIR}/hosts/${HOST}/disko-config.nix" > "$TMP_DISKO"

# --- 1. Partition & mount ---
sudo nix --experimental-features "nix-command flakes" \
  run github:nix-community/disko -- --mode disko "$TMP_DISKO"

# --- 2. Copy config ---
sudo mkdir -p /mnt/etc/nixos
sudo cp -r "${SCRIPT_DIR}"/. /mnt/etc/nixos/

# --- 3. Decrypt SSH private key ---
ENCRYPTED_KEY="${SCRIPT_DIR}/secrets/id_ed25519.age"

if [[ ! -f "$ENCRYPTED_KEY" ]]; then
  echo "ERROR: ${ENCRYPTED_KEY} not found."
  echo "Run once on a machine that has the key:"
  echo "  age -p -o secrets/id_ed25519.age ~/.ssh/id_ed25519"
  exit 1
fi

TMP_KEY="$(mktemp)"
trap 'rm -f "$TMP_DISKO" "$TMP_KEY"' EXIT

echo ""
echo "==> Decrypting SSH private key (enter your password):"
if command -v age &>/dev/null; then
  age -d -o "$TMP_KEY" "$ENCRYPTED_KEY"
else
  nix --extra-experimental-features "nix-command flakes" run nixpkgs#age -- -d -o "$TMP_KEY" "$ENCRYPTED_KEY"
fi

sudo mkdir -p /mnt/home/xm1k/.ssh
sudo cp "$TMP_KEY" /mnt/home/xm1k/.ssh/id_ed25519
sudo chmod 700 /mnt/home/xm1k/.ssh
sudo chmod 600 /mnt/home/xm1k/.ssh/id_ed25519

# Derive public key from private key
if command -v ssh-keygen &>/dev/null; then
  ssh-keygen -y -f "$TMP_KEY" | sudo tee /mnt/home/xm1k/.ssh/id_ed25519.pub > /dev/null
fi

# --- 4. Install ---
echo ""
echo "==> Installing NixOS for host: ${HOST}"
sudo nixos-install --flake "${SCRIPT_DIR}#${HOST}"

# --- 5. Fix ownership (xm1k = UID 1000, users = GID 100) ---
sudo chown -R 1000:100 /mnt/home/xm1k/.ssh

echo ""
echo "==> Done. You can reboot now."
