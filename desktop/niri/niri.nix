{ config, pkgs, lib, ... }:

{
  environment.systemPackages = with pkgs; [
    alacritty
    fuzzel
    waybar
    mako
  ];

  programs.niri.enable = true;

  home-manager.users.xm1k = {
    home.file.".config/niri/config.kdl" = {
      source = ./config.kdl;
      force  = true;
    };
  };

}