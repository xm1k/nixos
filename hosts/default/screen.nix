{ pkgs, pkgs-unstable, ... }:

{
  services.dbus.enable = true;
  programs.niri.enable = true;
  # niri 26.04 + SHM (memfd) fallback для screencast: нативный захват
  # Discord не умеет DMA-BUF, без патча демонстрация сразу закрывается.
  # Патч: https://github.com/rucnyz/niri/commit/f3207c77095114f9f2202ca7db8333d4c1a958d1
  programs.niri.package = pkgs-unstable.niri.overrideAttrs (old: {
    patches = (old.patches or [ ]) ++ [
      ../../modules/desktop/niri/niri-shm-screencast.patch
    ];
  });

  environment.variables = {
    GTK_CSD = "0";
    _JAVA_AWT_WM_NONREPARENTING = "1";
  };  
}
