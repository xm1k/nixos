{ config, pkgs, lib, ... }:

{
  environment.systemPackages = with pkgs; [
    alacritty
    fuzzel
    waybar
    mako
  ];

  programs.niri.enable = true;

  environment.etc."niri/config.kdl".text = builtins.readFile ./config.kdl;

}