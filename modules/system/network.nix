# Network configuration module
{ config, lib, pkgs, ... }:

{
  services.resolved = {
    enable = true;
    fallbackDns = [ "1.1.1.1" "8.8.8.8" ];
  };

  networking = {
    extraHosts = ''
      192.168.31.63 b.net
    '';
    networkmanager = {
      enable = true;
      dns = "systemd-resolved";
    };
    nameservers = [ "1.1.1.1" "8.8.8.8" ];
  };

  # Disable firewall for now
  networking.firewall.enable = false;
}