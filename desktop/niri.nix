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
    theme=dark
    icon_size=32
    position=top-right
    spacing=10
    opacity=0.85
    animations=true
    auto_hide=false
  '';

}