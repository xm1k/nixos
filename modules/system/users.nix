# User management configuration module
{ config, lib, pkgs, ... }:

{
  # Define a user account. Don't forget to set a password with 'passwd'.
  users.groups.battery_ctl = {};

  users.users.xm1k = {
    isNormalUser = true;
    extraGroups = [ "wheel" "networkmanager" "video" "battery_ctl" "input" ];
    initialPassword = "password";
  };
}