{ pkgs, ... }:

let
  store = "/var/lib/dns-names/hosts";

  dns = pkgs.writeShellApplication {
    name = "dns";
    runtimeInputs = [ pkgs.coreutils pkgs.gawk pkgs.util-linux ];
    text = ''
      STORE=${store}

      usage() {
        echo "dns add ИМЯ IP | dns rm ИМЯ... | dns ls" >&2
        exit 1
      }

      apply() {
        cat /etc/static/hosts "$STORE" > /etc/hosts.new
        mv /etc/hosts.new /etc/hosts
      }

      drop() {
        awk -v n="$1" '$2 != n' "$STORE" > "$STORE.new"
        mv "$STORE.new" "$STORE"
      }

      case "''${1:-ls}" in
        add|rm)
          if [ "$(id -u)" != 0 ]; then
            exec sudo "$0" "$@"
          fi
          ;;
      esac

      case "''${1:-ls}" in
        add)
          [ $# = 3 ] || usage
          drop "$2"
          echo "$3 $2" >> "$STORE"
          apply
          ;;
        rm)
          [ $# -ge 2 ] || usage
          shift
          for name in "$@"; do
            drop "$name"
          done
          apply
          ;;
        ls)
          column -t "$STORE"
          ;;
        *)
          usage
          ;;
      esac
    '';
  };
in
{
  environment.systemPackages = [ dns ];

  # /etc/hosts обычным файлом, а не ссылкой в store: dns дописывает туда свои имена
  environment.etc.hosts.mode = "0644";

  system.activationScripts.dns-names = {
    deps = [ "etc" ];
    text = ''
      mkdir -p /var/lib/dns-names
      touch ${store}
      cat /etc/static/hosts ${store} > /etc/hosts
    '';
  };
}
