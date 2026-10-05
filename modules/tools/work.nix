{ config, pkgs, ... }:

let
  work-vpn = pkgs.writeShellScriptBin "work-vpn" ''
    sudo podman stop ngate-root 2>/dev/null || true
    sudo podman start ngate-root

    sudo podman exec ngate-root ln -sf /dev/net/tun /dev/tun 2>/dev/null || true

    sleep 2

    sudo podman exec -d ngate-root sh -c "
      /opt/cprongate/ngatetun > /dev/null 2>&1 &
      sleep 1
      /opt/cprongate/ngateconsoleclient \
        -u da.smirnov \
        -p '$WORK_PASS' \
        -v https://nn.gw.yadro.com -I > /dev/null 2>&1 &
    "

    for i in $(seq 1 60); do
      ${pkgs.iproute2}/bin/ip link show tun0 >/dev/null 2>&1 && break
      sleep 1
    done

    if ! ${pkgs.iproute2}/bin/ip link show tun0 >/dev/null 2>&1; then
      echo "work-vpn: tun0 не поднялся за 60 секунд" >&2
      exit 1
    fi

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
