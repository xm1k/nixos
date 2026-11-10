{ config, pkgs, ... }:

# Автообновление Vault-токена (LDAP, da.smirnov / workpass).
#   - токен лежит в ~/.vault-token (его же читает vault CLI);
#   - таймер раз в час: токен жив и TTL > 3 дней → ничего; иначе renew; не вышло → новый логин;
#   - без VPN (нет tun0) тихо выходит, а после `work-vpn` стартует сам;
#   - в интерактивном bash $VAULT_TOKEN перечитывается из файла перед каждым промптом,
#     так что плейбуки, запущенные из shell, всегда видят свежий токен.
#   `vault-refresh` — обновить руками, `vault-refresh -f` — принудительный перелогин.
let
  vaultAddr = "https://re-vault.atmosphere.yadro.com";
  vaultHost = "re-vault.atmosphere.yadro.com";
  user = "da.smirnov";
  passFile = "/run/agenix/workpass";
  minTtl = 3 * 86400;

  vault-refresh = pkgs.writeShellApplication {
    name = "vault-refresh";
    runtimeInputs = [ pkgs.curl pkgs.jq pkgs.iproute2 pkgs.coreutils ];
    text = ''
      addr="${vaultAddr}"
      tokfile="$HOME/.vault-token"
      force=0; [ "''${1:-}" = "-f" ] && force=1

      # Vault доступен только через tun0 и не через корпоративный прокси
      if ! ip link show tun0 >/dev/null 2>&1; then
        echo "vault-refresh: нет VPN (tun0), пропускаю" >&2
        exit 0
      fi
      api() { curl -sS -m 15 --noproxy '*' "$@"; }

      save() {
        umask 077
        printf '%s' "$1" > "$tokfile.tmp"
        mv -f "$tokfile.tmp" "$tokfile"
      }

      if [ "$force" = 0 ] && [ -s "$tokfile" ]; then
        tok=$(tr -d '[:space:]' < "$tokfile")
        ttl=$(api -H "X-Vault-Token: $tok" "$addr/v1/auth/token/lookup-self" 2>/dev/null | jq -r '.data.ttl // 0' 2>/dev/null || echo 0)
        ttl=''${ttl:-0}
        if [ "$ttl" -gt ${toString minTtl} ]; then
          echo "vault-refresh: токен жив, TTL $((ttl / 86400)) д." >&2
          exit 0
        fi
        if [ "$ttl" -gt 0 ]; then
          ttl=$(api -X POST -H "X-Vault-Token: $tok" "$addr/v1/auth/token/renew-self" 2>/dev/null | jq -r '.auth.lease_duration // 0' 2>/dev/null || echo 0)
          ttl=''${ttl:-0}
          if [ "$ttl" -gt ${toString minTtl} ]; then
            echo "vault-refresh: продлён, TTL $((ttl / 86400)) д." >&2
            exit 0
          fi
        fi
      fi

      # Пароль уходит через stdin, в аргументы/ps не попадает
      resp=$(jq -n --rawfile p ${passFile} '{password: ($p | rtrimstr("\n"))}' \
        | api -X POST --data @- "$addr/v1/auth/ldap/login/${user}")
      tok=$(jq -r '.auth.client_token // empty' <<<"$resp")
      if [ -z "$tok" ]; then
        echo "vault-refresh: логин не удался: $(jq -c '.errors' <<<"$resp" 2>/dev/null || echo "$resp")" >&2
        exit 1
      fi
      save "$tok"
      echo "vault-refresh: новый токен, TTL $(( $(jq -r '.auth.lease_duration' <<<"$resp") / 86400 )) д." >&2
    '';
  };
in
{
  home.packages = [ vault-refresh ];

  home.sessionVariables.VAULT_ADDR = vaultAddr;

  systemd.user.services.vault-refresh = {
    Unit.Description = "Refresh Vault token (LDAP)";
    Service = {
      Type = "oneshot";
      ExecStart = "${vault-refresh}/bin/vault-refresh";
    };
  };

  systemd.user.timers.vault-refresh = {
    Unit.Description = "Refresh Vault token hourly";
    Timer = {
      OnStartupSec = "2min";
      OnUnitActiveSec = "1h";
      Persistent = true;
    };
    Install.WantedBy = [ "timers.target" ];
  };

  programs.bash.initExtra = ''
    export VAULT_ADDR="${vaultAddr}"
    # Vault не ходит через корпоративный прокси
    case ",''${no_proxy:-}," in *",${vaultHost},"*) ;; *)
      export no_proxy="''${no_proxy:+$no_proxy,}${vaultHost}"
      export NO_PROXY="$no_proxy" ;;
    esac
    # read — встроенная команда bash, без fork/exec; файл без \n в конце → код 1, но значение прочитано
    __vault_token_sync() {
      local t
      { read -r t || [ -n "$t" ]; } < "$HOME/.vault-token" 2>/dev/null && export VAULT_TOKEN="$t"
      return 0
    }
    __vault_token_sync
    PROMPT_COMMAND="__vault_token_sync''${PROMPT_COMMAND:+;$PROMPT_COMMAND}"
  '';
}
