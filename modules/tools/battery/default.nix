{ pkgs, ... }:

let
  script = pkgs.writeText "battery.py" (builtins.readFile ./battery.py);
  battery = pkgs.writeShellApplication {
    name = "battery";
    text = ''
      exec ${pkgs.python3}/bin/python3 ${script} --tlp ${pkgs.tlp}/bin/tlp "$@"
    '';
  };
in
{
  environment.systemPackages = [ battery ];
}
