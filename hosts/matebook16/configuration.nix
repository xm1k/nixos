{ config, lib, pkgs, inputs, ... }:

{

  imports =
    [
      ./hardware-configuration.nix
      ./screen.nix
    ];

  networking.hostName = "matebook16";

  services.logind.lidSwitch = "ignore";

  # Wake on WLAN — Intel Alder Lake CNVi (phy0, PCI 0000:00:14.3, ACPI CNVW)
  # MAC для отправки magic packet: 10:f6:0a:35:65:dd
  systemd.services.wowlan = {
    description = "Enable Wake on WLAN (magic packet)";
    after = [ "sys-subsystem-net-devices-wlp0s20f3.device" ];
    wantedBy = [ "multi-user.target" "sleep.target" ];
    before = [ "sleep.target" ];
    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
      ExecStart = pkgs.writeShellScript "enable-wowlan" ''
        grep "^CNVW" /proc/acpi/wakeup | grep -q "disabled" && echo CNVW > /proc/acpi/wakeup || true
        echo enabled > /sys/bus/pci/devices/0000:00:14.3/power/wakeup
        ${pkgs.iw}/bin/iw phy phy0 wowlan enable magic-packet
      '';
    };
  };

}


