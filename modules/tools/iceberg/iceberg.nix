{ pkgs, ... }:

let
  work-vpn = pkgs.writeShellScriptBin "ice" ''
    python /etc/nixos/modules/tools/iceberg/vault.py "$@"
  '';
in
{
  home.packages = [
    work-vpn
  ];
}
