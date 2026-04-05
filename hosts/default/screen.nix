{ pkgs, ... }:

{
  services.dbus.enable = true;
  programs.niri.enable = true;

  environment.variables = {
    GTK_CSD = "0";
    _JAVA_AWT_WM_NONREPARENTING = "1";
  };  
}
