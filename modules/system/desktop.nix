# Desktop environment configuration module
{ config, lib, pkgs, ... }:

{
  services.xserver = {
    enable = true;
    xkb = {
      layout = "us,ru";
      variant = ",";
      options = "grp:alt_shift_toggle";
    };
  };

  services.displayManager.sddm.enable = true;
  services.displayManager.autoLogin.enable = true;
  services.displayManager.autoLogin.user = "xm1k";
  services.displayManager.defaultSession = "niri";

  services.getty.autologinUser = "xm1k";
  services.upower.enable = true;

  programs.niri.enable = true;
}