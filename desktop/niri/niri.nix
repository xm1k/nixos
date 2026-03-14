{ config, pkgs, lib, ... }:

{
  environment.systemPackages = with pkgs; [
		xwayland
		xwayland-satellite
		xorg.xhost
		swww
  ];

  programs.niri.enable = true;

  home-manager.users.xm1k = {
    imports = [./cursor.nix];
		home.file.".config/niri/config.kdl" = {
      source = ./config.kdl;
      force  = true;
    };
  };	
	
}
