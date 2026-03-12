{ config, pkgs, lib, ... }:

let
  niriConfigFile = ./config.kdl;
in

{
  environment.systemPackages = with pkgs; [
    alacritty
    fuzzel
    waybar
    mako
  ];

  programs.niri.enable = true;

  home-manager.users.root = {
    home.stateVersion = "25.11";
    home.file.".config/niri/config.kdl".source = ./config.kdl;
  };

}