# Hardware configuration module
{ config, lib, pkgs, ... }:

{
  hardware = {
    bluetooth = {
      enable = true;
      powerOnBoot = true;
      settings = {
        General = {
          Enable = "Source,Sinc,Media,Socket";
          Experimental = true;
        };
      };
    };
    graphics = {
      enable = true;
      extraPackages = with pkgs; [
        intel-media-driver
        libvdpau-va-gl
      ];
    };
  };

  services.thermald.enable = true;

  services.tlp = {
    enable = true;
    settings = {
      GPU_SCALING_GOVERNOR_ON_AC = "performance";
      GPU_SCALING_GOVERNOR_ON_BAT = "powersave";

      PCIE_ASPM_ON_BAT = "powersupersave";
      INTEL_GPU_MIN_FREQ_ON_BAT = 300;
      INTEL_GPU_MAX_FREQ_ON_BAT = 800;
      INTEL_GPU_BOOST_ON_BAT = 0;

      CPU_ENERGY_PERF_POLICY_ON_AC = "performance";
      CPU_ENERGY_PERF_POLICY_ON_BAT = "power";

      CPU_BOOST_ON_AC = 1;
      CPU_BOOST_ON_BAT = 0;
    };
  };

  services.power-profiles-daemon.enable = false;
}