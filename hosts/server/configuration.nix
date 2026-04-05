{ config, lib, pkgs, inputs, ... }:

{	

  imports =
    [
      ./hardware-configuration.nix
    ];

  networking.hostName = "desktop";

  virtualisation.podman = {
    enable = true;
    dockerCompat = true;
    enableNvidia = true; 
  };

  hardware.nvidia-container-toolkit.enable = true;

}


