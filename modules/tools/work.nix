{ config, pkgs, ... }:

let
  work-vpn = pkgs.writeShellScriptBin "work-vpn" ''
    ${pkgs.distrobox}/bin/distrobox stop -Y ngate-root --root 2>/dev/null
    ${pkgs.distrobox}/bin/distrobox enter ngate-root --root -- sh -c "
      sudo /opt/cprongate/ngatetun > /dev/null 2>&1 &
      sleep 1
      sudo /opt/cprongate/ngateconsoleclient \
        -u da.smirnov \
        -p '$WORK_PASS' \
        -v \
        https://nn.gw.yadro.com > /dev/null 2>&1 &
      sleep 15"

    sudo resolvectl dns tun0 172.31.129.43
    sudo resolvectl domain tun0 "~yadro.com"
    
    pkill quickshell || true

    export HTTPS_PROXY="http://da.smirnov:$FREE_PORT_PASS@10.1.252.242:9999"
    export http_proxy="http://da.smirnov:$FREE_PORT_PASS@10.1.252.242:9999"
    
    noctalia-shell > /dev/null 2>&1 & disown
  '';
in
{
  home.packages = [
    work-vpn
    pkgs.mailspring
  ];
}
