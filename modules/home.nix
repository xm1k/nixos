# Home Manager configuration for user xm1k
{ config, pkgs, inputs, lib, ... }:

{
  # xresources.properties = lib.mkForce {};
  home.pointerCursor = {
    x11.enable = false;
    gtk.enable = true;
  };

  home.stateVersion = "25.11";

  home.sessionVariables = {
    GTK_CSD = "0";
    
    _JAVA_AWT_WM_NONREPARENTING = "1";
    
    QT_WAYLAND_DISABLE_WINDOWDECORATION = "1";
    
    QT_QPA_PLATFORM = "wayland;xcb"; 
  };

  imports = [
    ./desktop/firefox/firefox.nix
    ./tools/python.nix
    ./tools/k8s.nix
    ./tools/iceberg/iceberg.nix
    ./desktop/niri/niri.nix
    ./desktop/nixvim/vim.nix
    ./tools/vscode.nix
		./tools/work.nix
    ./desktop/starship/starship.nix
    ./desktop/noctalia/noctalia.nix
		./tools/podman/podman.nix
    ./games/core.nix
    inputs.nixvim.homeModules.nixvim
  ];
}
