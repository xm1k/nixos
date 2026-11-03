{ pkgs, ... }:

let
  # Находит активную wayland-сессию: печатает "<uid> <socket>".
  findSession = ''
    find_session() {
      for sock in /run/user/*/wayland-[0-9]; do
        [ -S "$sock" ] || continue
        uid=$(stat -c %u "$sock")
        if [ "$(id -u)" = 0 ] || [ "$(id -u)" = "$uid" ]; then
          echo "$uid $sock"
          return 0
        fi
      done
      return 1
    }
  '';

  # lock — блокирует экран (в т.ч. из ssh-сессии).
  lock = pkgs.writeShellScriptBin "lock" ''
    ${findSession}

    if ${pkgs.procps}/bin/pgrep -x hyprlock >/dev/null; then
      echo "lock: already locked"
      exit 0
    fi

    read -r uid sock < <(find_session) || { echo "lock: no wayland session found" >&2; exit 1; }
    user=$(id -nu "$uid")
    runtime=$(dirname "$sock")

    cmd=(env XDG_RUNTIME_DIR="$runtime" WAYLAND_DISPLAY="$(basename "$sock")" \
      ${pkgs.util-linux}/bin/setsid -f ${pkgs.hyprlock}/bin/hyprlock)
    if [ "$(id -u)" = 0 ] && [ "$uid" != 0 ]; then
      ${pkgs.util-linux}/bin/runuser -u "$user" -- "''${cmd[@]}" >/dev/null 2>&1
    else
      "''${cmd[@]}" >/dev/null 2>&1
    fi
    echo "lock: screen locked ($user)"
  '';

  # unlock — снимает блокировку (hyprlock выходит по SIGUSR1).
  unlock = pkgs.writeShellScriptBin "unlock" ''
    if ${pkgs.procps}/bin/pkill -USR1 -x hyprlock; then
      echo "unlock: screen unlocked"
    else
      echo "unlock: hyprlock is not running (or no permission)" >&2
      exit 1
    fi
  '';

  # wake-up <ip> [mac] — Wake-on-LAN. MAC берётся из аргумента, ARP-таблицы
  # или кэша ~/.cache/wake-up/macs (заполняется, пока машина в сети).
  wake-up = pkgs.writeShellScriptBin "wake-up" ''
    set -e
    ip_addr="$1"
    mac="$2"
    cache="''${XDG_CACHE_HOME:-$HOME/.cache}/wake-up/macs"

    if [ -z "$ip_addr" ]; then
      echo "usage: wake-up <ip> [mac]" >&2
      exit 1
    fi

    if [ -z "$mac" ]; then
      mac=$(${pkgs.iproute2}/bin/ip neigh show "$ip_addr" \
        | ${pkgs.gawk}/bin/awk '/lladdr/ { for (i = 1; i < NF; i++) if ($i == "lladdr") print $(i + 1) }' \
        | head -n1)
    fi
    if [ -z "$mac" ] && [ -f "$cache" ]; then
      mac=$(${pkgs.gawk}/bin/awk -v ip="$ip_addr" '$1 == ip { print $2 }' "$cache" | tail -n1)
    fi
    if [ -z "$mac" ]; then
      echo "wake-up: MAC for $ip_addr unknown, pass it: wake-up $ip_addr <mac>" >&2
      exit 1
    fi

    mkdir -p "$(dirname "$cache")"
    touch "$cache"
    ${pkgs.gnused}/bin/sed -i "/^$ip_addr /d" "$cache"
    echo "$ip_addr $mac" >> "$cache"

    # Широковещательный адрес подсети, через которую виден ip_addr.
    dev=$(${pkgs.iproute2}/bin/ip -o route get "$ip_addr" \
      | ${pkgs.gawk}/bin/awk '{ for (i = 1; i < NF; i++) if ($i == "dev") print $(i + 1) }')
    brd=$(${pkgs.iproute2}/bin/ip -o -4 addr show dev "$dev" \
      | ${pkgs.gawk}/bin/awk '{ for (i = 1; i < NF; i++) if ($i == "brd") print $(i + 1) }' \
      | head -n1)

    for target in ''${brd:-} 255.255.255.255; do
      ${pkgs.wakeonlan}/bin/wakeonlan -i "$target" "$mac" >/dev/null
    done
    echo "wake-up: magic packet sent to $mac ($ip_addr via ''${brd:-255.255.255.255})"
  '';
in
{
  environment.systemPackages = [ lock unlock wake-up ];
}
