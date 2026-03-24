{ config, pkgs, lib, ... }:

{
  imports = [./cursor.nix];

  home.packages = with pkgs; [
		xwayland
		xwayland-satellite
		xorg.xhost
		swww
  ];

  home.file.".config/niri/config.kdl" = {
    source = ./config.kdl;
    force  = true;
  };

}
