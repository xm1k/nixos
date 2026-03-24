# Niri-specific Home Manager configuration
{ config, pkgs, inputs, ... }:

{
  home-manager.users.xm1k = {
    imports = [../../desktop/niri/cursor.nix];
    home.file.".config/niri/config.kdl" = {
      source = ../../desktop/niri/config.kdl;
      force = true;
    };
  };
}