# Home Manager configuration for user xm1k
{ config, pkgs, inputs, ... }:

{
  home.stateVersion = "25.11";
  
  imports = [
    ../../desktop/firefox/firefox.nix
    ../../tools/python.nix
  ];

  # Home Manager specific configurations for niri
  home-manager.users.xm1k = {
    imports = [../../desktop/niri/cursor.nix];
    home.file.".config/niri/config.kdl" = {
      source = ../../desktop/niri/config.kdl;
      force = true;
    };
  };
}