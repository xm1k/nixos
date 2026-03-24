# System packages configuration module
{ config, lib, pkgs, ... }:

{
  # List packages installed in system profile.
  environment.systemPackages = with pkgs; [
    vivaldi
    wget
    curl
    git
    python310
    docker
    wl-clipboard
    fastfetch
    unzip
    unrar
    mpv
    ansible
    byedpi
    obsidian
    k9s
    dnsutils
    nautilus
    steam-run
    brightnessctl
    bottom
    prismlauncher
    onlyoffice-desktopeditors
  ];

  # Udev rules for battery control
  services.udev.extraRules = ''
    SUBSYSTEM=="power_supply", KERNEL=="BAT*", \
      RUN+="${pkgs.coreutils}/bin/chgrp battery_ctl /sys%p/charge_control_end_threshold", \
      RUN+="${pkgs.coreutils}/bin/chmod g+w /sys%p/charge_control_end_threshold"
  '';
}