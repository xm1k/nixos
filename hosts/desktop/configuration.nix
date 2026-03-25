{ config, lib, pkgs, inputs, ... }:

{	

  imports =
    [
      ./hardware-configuration.nix
    ];

  networking.hostName = "desktop";

}


