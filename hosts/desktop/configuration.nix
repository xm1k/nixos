{ config, lib, pkgs, inputs, ... }:

{	

  imports =
    [
      ./hardware-configuration.nix
      ./screen.nix
    ];

  networking.hostName = "matebook16";

}


