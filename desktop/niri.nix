{ config, pkgs, lib, ... }:

{
  environment.systemPackages = with pkgs; [
    alacritty
    fuzzel
    waybar
    mako
  ];

  programs.niri.enable = true;

  environment.etc."niri/config".text = ''
    theme=default
    icon_size=24
    position=top-right
  '';
  
}