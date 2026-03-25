sudo nix --experimental-features "nix-command flakes" run github:nix-community/disko -- --mode disko ./hosts/nixos/disko-config.nix
sudo mkdir -p /mnt/etc/nixos
sudo cp -r ../nixos* /mnt/etc/
sudo nixos-install --flake .#nixos
