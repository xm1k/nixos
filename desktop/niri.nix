{ config, pkgs, lib, ... }:

{
  environment.systemPackages = with pkgs; [
    alacritty
    fuzzel
    waybar
    mako
  ];

  programs.niri.enable = true;

  programs.niri.settings = {
    theme = "default";
    iconSize = 24;
    position = "top-right";
  };
}