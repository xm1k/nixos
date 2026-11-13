{ config, lib, pkgs, ... }:

# Точка доступа b.net: ноут раздаёт свой Wi-Fi (ap0 поверх того же радио),
# подключившиеся ходят в интернет через wlan/eth ноута, а *.b.net — через tailscale0
# только до berry. В tun0 (рабочий VPN) и в остальной tailnet — никогда.
#
#   bnet-ap on | off | status | qr | log
#
# Ограничение железа: точка доступа и клиент на одном радио — один канал. Точка
# встаёт на канал сети, к которой подключён ноут; DFS-каналы 5 ГГц (52–144) не подходят.

let
  cfg = config.services.bnetAp;
  ap = "ap0";
  addr = "${cfg.subnet}.1";
  stateDir = "/run/bnet-ap";

  path = lib.makeBinPath [ pkgs.iw pkgs.iproute2 pkgs.nftables pkgs.procps pkgs.gawk pkgs.coreutils ];

  # Аплинки — правилом на каждый, nft не везде понимает маски в наборах
  uplinkRules = lib.concatMapStringsSep "\n    "
    (i: ''iifname "${ap}" oifname "${i}" accept'') cfg.uplinks;

  rules = pkgs.writeText "bnet-ap.nft" ''
    table inet bnet_ap {
      chain input {
        type filter hook input priority filter; policy accept;
        iifname "${ap}" ct state established,related accept
        iifname "${ap}" udp dport { 53, 67 } accept
        iifname "${ap}" tcp dport 53 accept
        iifname "${ap}" icmp type echo-request accept
        iifname "${ap}" drop
      }

      chain forward {
        type filter hook forward priority filter; policy accept;
        # MTU tailscale 1280 — режем MSS, иначе крупные ответы b.net застревают
        iifname "${ap}" oifname "tailscale0" tcp flags syn tcp option maxseg size set rt mtu
        # DNS-over-TLS отклоняем: Android тогда спрашивает наш DNS и видит b.net
        iifname "${ap}" tcp dport 853 reject with tcp reset
        iifname "${ap}" oifname "tailscale0" ip daddr ${cfg.berryIp} accept
        ${uplinkRules}
        # всё прочее (tun0, остальная tailnet, docker, …) — нет
        iifname "${ap}" drop
        oifname "${ap}" ct state established,related accept
        oifname "${ap}" drop
      }

      chain prerouting {
        type nat hook prerouting priority dstnat; policy accept;
        # Прописанный руками DNS (8.8.8.8 и т.п.) всё равно попадает в наш dnsmasq
        iifname "${ap}" meta l4proto { tcp, udp } th dport 53 redirect to :53
      }

      chain postrouting {
        type nat hook postrouting priority srcnat; policy accept;
        ip saddr ${cfg.subnet}.0/24 oifname != "${ap}" masquerade
      }
    }
  '';

  dnsmasqConf = pkgs.writeText "bnet-ap-dnsmasq.conf" ''
    interface=${ap}
    bind-dynamic
    except-interface=lo
    no-resolv
    no-hosts
    domain-needed
    server=/b.net/${cfg.berryIp}
    ${lib.concatMapStringsSep "\n" (d: "address=/${d}/") cfg.dnsBlock}
    # Остальное — через resolved ноута (DNS текущей сети)
    server=127.0.0.53
    dhcp-authoritative
    dhcp-range=${cfg.subnet}.10,${cfg.subnet}.250,255.255.255.0,12h
    dhcp-option=option:router,${addr}
    dhcp-option=option:dns-server,${addr}
    dhcp-leasefile=/run/bnet-ap-dns/leases
  '';

  up = pkgs.writeShellScript "bnet-ap-up" ''
    set -euo pipefail
    export PATH=${path}
    phy_if=${cfg.interface}

    pass=$(tr -d '\n' < ${cfg.passwordFile})
    if [ ''${#pass} -lt 8 ] || [ ''${#pass} -gt 63 ]; then
      echo "bnet-ap: пароль Wi-Fi должен быть 8–63 символа" >&2
      exit 1
    fi

    # Канал — тот же, что у клиентского подключения (одно радио)
    ch=$(iw dev "$phy_if" info | awk '$1 == "channel" { print $2; exit }')
    ch=''${ch:-6}
    if [ "$ch" -le 14 ]; then
      mode=g
    elif { [ "$ch" -ge 36 ] && [ "$ch" -le 48 ]; } || [ "$ch" -ge 149 ]; then
      mode=a
    else
      echo "bnet-ap: $phy_if на DFS-канале $ch, точку доступа на нём поднять нельзя." >&2
      echo "bnet-ap: подключись к сети на 2.4 ГГц или на 5 ГГц с каналом 36–48 / 149–165." >&2
      exit 1
    fi

    iw dev ${ap} del 2>/dev/null || true
    iw dev "$phy_if" interface add ${ap} type __ap
    # Свой MAC (бит locally administered), иначе iwlwifi не даст два интерфейса
    mac=$(cat /sys/class/net/"$phy_if"/address)
    ip link set ${ap} address "$(printf '%02x' $(( 0x''${mac%%:*} ^ 0x02 ))):''${mac#*:}"
    sysctl -qw net.ipv6.conf.${ap}.disable_ipv6=1
    ip addr flush dev ${ap}
    ip addr add ${addr}/24 dev ${ap}

    cat /proc/sys/net/ipv4/ip_forward > ${stateDir}/ip_forward
    nft delete table inet bnet_ap 2>/dev/null || true
    nft -f ${rules}
    sysctl -qw net.ipv4.ip_forward=1

    umask 077
    {
      echo "interface=${ap}"
      echo "driver=nl80211"
      echo "ctrl_interface=${stateDir}/hostapd"
      echo "ssid=${cfg.ssid}"
      echo "hw_mode=$mode"
      echo "channel=$ch"
      echo "ieee80211n=1"
      [ "$mode" = a ] && echo "ieee80211ac=1"
      echo "wmm_enabled=1"
      echo "auth_algs=1"
      echo "wpa=2"
      echo "wpa_key_mgmt=WPA-PSK"
      echo "rsn_pairwise=CCMP"
      echo "wpa_passphrase=$pass"
    } > ${stateDir}/hostapd.conf
    echo "bnet-ap: ${cfg.ssid} на канале $ch ($mode), сеть ${cfg.subnet}.0/24"
  '';

  down = pkgs.writeShellScript "bnet-ap-down" ''
    export PATH=${path}
    nft delete table inet bnet_ap 2>/dev/null
    iw dev ${ap} del 2>/dev/null
    if [ -r ${stateDir}/ip_forward ]; then
      sysctl -qw net.ipv4.ip_forward="$(cat ${stateDir}/ip_forward)"
    fi
    true
  '';

  cli = pkgs.writeShellApplication {
    name = "bnet-ap";
    runtimeInputs = [ pkgs.systemd pkgs.coreutils pkgs.gawk pkgs.qrencode pkgs.hostapd ];
    text = ''
      unit=bnet-ap.service

      clients() {
        if [ -r /run/bnet-ap-dns/leases ] && [ -s /run/bnet-ap-dns/leases ]; then
          echo "клиенты:"
          awk '{ printf "  %-15s %-20s %s\n", $3, ($4 == "*" ? "-" : $4), $2 }' /run/bnet-ap-dns/leases
        else
          echo "клиентов нет"
        fi
      }

      case "''${1:-status}" in
        on)
          sudo systemctl start "$unit"
          systemctl is-active -q "$unit" && echo "включено: ${cfg.ssid}, сертификат b.net: http://b.net/api/ca.crt"
          ;;
        off)
          sudo systemctl stop "$unit"
          echo "выключено"
          ;;
        status)
          if systemctl is-active -q "$unit"; then
            echo "включено: ${cfg.ssid}"
            clients
          else
            echo "выключено"
          fi
          ;;
        qr)
          # QR для подключения: камера телефона → «подключиться к сети»
          pass=$(sudo cat ${cfg.passwordFile} | tr -d '\n')
          qrencode -t ansiutf8 "WIFI:T:WPA;S:${cfg.ssid};P:$pass;;"
          echo "${cfg.ssid} / сертификат: http://b.net/api/ca.crt"
          ;;
        log)
          journalctl -u "$unit" -u bnet-ap-dns.service -n 50 --no-pager
          ;;
        *)
          echo "bnet-ap on | off | status | qr | log" >&2
          exit 1
          ;;
      esac
    '';
  };
in
{
  options.services.bnetAp = {
    enable = lib.mkEnableOption "точка доступа Wi-Fi с выходом в b.net через tailscale";

    autostart = lib.mkOption {
      type = lib.types.bool;
      default = false;
      description = "Поднимать точку при загрузке (иначе — `bnet-ap on`).";
    };

    interface = lib.mkOption {
      type = lib.types.str;
      default = "wlp0s20f3";
      description = "Wi-Fi-интерфейс, на радио которого создаётся ap0.";
    };

    uplinks = lib.mkOption {
      type = lib.types.listOf lib.types.str;
      default = [ "wlp0s20f3" "en*" "eth*" ];
      description = "Куда пускать трафик в интернет. tun0 сюда не добавлять.";
    };

    ssid = lib.mkOption {
      type = lib.types.str;
      default = "b.net";
    };

    passwordFile = lib.mkOption {
      type = lib.types.path;
      description = "Файл с паролем Wi-Fi (8–63 символа), обычно секрет agenix.";
    };

    subnet = lib.mkOption {
      type = lib.types.str;
      default = "192.168.77";
      description = "Первые три октета сети /24 для клиентов.";
    };

    berryIp = lib.mkOption {
      type = lib.types.str;
      default = "100.67.43.44";
      description = "Tailscale-IP berry: DNS b.net и единственный адрес tailnet, доступный клиентам.";
    };

    dnsBlock = lib.mkOption {
      type = lib.types.listOf lib.types.str;
      default = [ ];
      description = "Домены, которые для клиентов не резолвятся (чтобы их запросы не уходили в DNS VPN).";
    };
  };

  config = lib.mkIf cfg.enable {
    networking.networkmanager.unmanaged = [ "interface-name:${ap}" ];

    environment.systemPackages = [ cli ];

    systemd.services.bnet-ap = {
      description = "b.net Wi-Fi access point";
      after = [ "network.target" "tailscaled.service" ];
      wants = [ "bnet-ap-dns.service" ];
      wantedBy = lib.mkIf cfg.autostart [ "multi-user.target" ];
      serviceConfig = {
        RuntimeDirectory = "bnet-ap";
        RuntimeDirectoryMode = "0700";
        ExecStartPre = up;
        ExecStart = "${pkgs.hostapd}/bin/hostapd ${stateDir}/hostapd.conf";
        ExecStopPost = down;
        # Ноут переподключился на другой канал — hostapd падает, поднимаемся заново
        Restart = "on-failure";
        RestartSec = 5;
      };
      startLimitIntervalSec = 120;
      startLimitBurst = 5;
    };

    systemd.services.bnet-ap-dns = {
      description = "b.net Wi-Fi access point: DHCP + DNS";
      bindsTo = [ "bnet-ap.service" ];
      partOf = [ "bnet-ap.service" ];
      after = [ "bnet-ap.service" ];
      serviceConfig = {
        RuntimeDirectory = "bnet-ap-dns";
        ExecStart = "${pkgs.dnsmasq}/bin/dnsmasq --keep-in-foreground --conf-file=${dnsmasqConf}";
        Restart = "on-failure";
        RestartSec = 2;
      };
    };
  };
}
