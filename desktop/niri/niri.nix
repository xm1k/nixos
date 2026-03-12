{ config, pkgs, lib, ... }:

{
  environment.systemPackages = with pkgs; [
    alacritty
    fuzzel
    waybar
    mako
  ];

  programs.niri.enable = true;

  system.activationScripts.niri-config = ''
    mkdir -p /home/root/.config/niri
    cp ${./desktop/niri/config.kdl} /home/root/.config/niri/config.kdl
  '';

}