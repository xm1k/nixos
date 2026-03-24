# Edit this configuration file to define what should be installed on
# your system. Help is available in the configuration.nix(5) man page, on
# https://search.nixos.org/options and in the NixOS manual (`nixos-help`).

{ config, lib, pkgs, inputs, ... }:

{	
  # Import all system modules
  imports = [
    # Include the results of the hardware scan.
    ./games/core.nix
    ./hardware-configuration.nix
    ./desktop/niri/niri.nix
    ./desktop/nixvim/vim.nix
    ./tools/vscode.nix
    ./tools/work.nix
    ./desktop/starship/starship.nix
    ./tools/podman/podman.nix
			./secrets/secrets-manager.nix
			./network/byedpi.nix

    # System modules
    ./modules/system/network.nix
    ./modules/system/hardware.nix
    ./modules/system/desktop.nix
    ./modules/system/services.nix
    ./modules/system/packages.nix
    ./modules/system/users.nix
    ./modules/system/misc.nix
    ];

  # Use the systemd-boot EFI boot loader.
  nixpkgs.config.allowUnfree = true;
  boot.loader.systemd-boot.enable = true;
  boot.loader.efi.canTouchEfiVariables = true;

  # networking.hostName = "nixos"; # Define your hostname.

  system.stateVersion = "25.11"; # Did you read the comment?
}

