{ config, pkgs, lib, ... }:

{
  environment.systemPackages = with pkgs; [
		xwayland
		xwayland-satellite
		xorg.xhost
		swww
  ];

  programs.niri.enable = true;
}

