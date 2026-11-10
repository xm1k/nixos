{ pkgs, ... }:

let
  # zj — выбор сессии: Enter войти / Ctrl-D удалить / новое имя + Enter создать
  zj = pkgs.writeShellApplication {
    name = "zj";
    runtimeInputs = [ pkgs.shpool pkgs.fzf pkgs.gawk ];
    text = ''
      if [ -n "''${SHPOOL_SESSION_NAME:-}" ]; then
        echo "Уже в сессии $SHPOOL_SESSION_NAME — сначала закройте окно (Alt+Q) или: shpool detach"; exit 1
      fi
      out=$(shpool list 2>/dev/null | fzf --print-query --reverse --height=40% \
        --header-lines=1 --prompt='session> ' \
        --header='Enter: войти · Ctrl-D: удалить · новое имя + Enter: создать' \
        --bind 'ctrl-d:execute-silent(shpool kill {1})+reload(shpool list 2>/dev/null)' \
        ) || true
      query=$(sed -n 1p <<<"$out")
      pick=$(sed -n 2p <<<"$out" | awk '{print $1}')
      name=''${pick:-$query}
      [ -z "$name" ] && exit 0
      exec shpool attach --force "$name"
    '';
  };

  # zj-here — превратить текущий shell в новую сессию (имя = текущий каталог)
  zj-here = pkgs.writeShellApplication {
    name = "zj-here";
    runtimeInputs = [ pkgs.shpool pkgs.gawk ];
    text = ''
      if [ -n "''${SHPOOL_SESSION_NAME:-}" ]; then
        echo "Уже в сессии $SHPOOL_SESSION_NAME"; exit 1
      fi
      if [ "$PWD" = "$HOME" ]; then base=home; else base=$(basename "$PWD"); fi
      base=$(tr -c 'A-Za-z0-9_.-' '-' <<<"$base" | sed 's/-*$//')
      existing=$(shpool list 2>/dev/null | awk 'NR>1 {print $1}')
      name=$base; i=2
      while grep -qx -- "$name" <<<"$existing"; do name="$base-$i"; i=$((i+1)); done
      exec shpool attach "$name"
    '';
  };
  # wc — обёртка над wl-copy:
  #   wc "текст" / cmd | wc  — как wl-copy (флаги wl-copy тоже проходят)
  #   wc                     — выбрать env-переменную в fzf и скопировать её значение
  #   wc -l / -w / -c / -m / -L … — обычный coreutils wc (чтобы `cmd | wc -l` не ломался)
  wclip = pkgs.writeShellApplication {
    name = "wclip";
    runtimeInputs = [ pkgs.wl-clipboard pkgs.fzf pkgs.coreutils ];
    text = ''
      case "''${1:-}" in
        -l|-w|-c|-m|-L|--lines|--words|--bytes|--chars|--max-line-length|--files0-from=*|--total=*)
          exec wc "$@" ;;
      esac

      if [ $# -gt 0 ] || [ ! -t 0 ]; then
        exec wl-copy "$@"
      fi

      name=$(env -0 | while IFS= read -r -d "" kv; do printf '%s\n' "''${kv%%=*}"; done | sort -u | fzf --reverse --height=40% --prompt='env> ') || exit 0
      [ -z "$name" ] && exit 0
      printenv -- "$name" | wl-copy -n
      echo "Скопировано: \$$name" >&2
    '';
  };
in
{
  home.packages = [ pkgs.shpool zj zj-here wclip ];

  # Только в интерактивном shell — скрипты продолжают видеть обычный wc
  programs.bash.shellAliases.wc = "wclip";

  # Демон shpool держит сессии; запускается по сокету при первом обращении
  systemd.user.sockets.shpool = {
    Unit.Description = "Shpool Shell Session Pooler";
    Socket = {
      ListenStream = "%t/shpool/shpool.socket";
      SocketMode = "0600";
    };
    Install.WantedBy = [ "sockets.target" ];
  };
  systemd.user.services.shpool = {
    Unit = {
      Description = "Shpool - Shell Session Pool";
      Requires = [ "shpool.socket" ];
    };
    Service = {
      Type = "simple";
      ExecStart = "${pkgs.shpool}/bin/shpool daemon";
      KillMode = "mixed";
      TimeoutStopSec = "2s";
      SendSIGHUP = true;
    };
  };

  # Никаких префиксов и приветствий — метку сессии показывает starship
  xdg.configFile."shpool/config.toml".text = ''
    prompt_prefix = ""
    motd = "never"
  '';

  # Alt+S — сделать текущий shell сессией, Alt+A — выбрать сессию.
  # Команда вставляется в строку и выполняется как обычная (пробел в начале — не попадёт в историю).
  programs.bash.historyControl = [ "ignoredups" "ignorespace" ];
  programs.bash.initExtra = ''
    if [[ $- == *i* && -z "''${SHPOOL_SESSION_NAME:-}" ]]; then
      bind '"\es": "\C-a\C-k zj-here\C-m"'
      bind '"\ea": "\C-a\C-k zj\C-m"'
    fi
  '';

  # zoxide: умный cd (запоминает каталоги). `cd nixos`, `cdi` — интерактивно
  programs.zoxide = {
    enable = true;
    options = [ "--cmd cd" ];
  };

  # fzf: Ctrl+R / Ctrl+T / Alt+C, и выбор сессий
  programs.fzf = {
    enable = true;
    defaultCommand = "fd --type f --hidden --exclude .git";
    fileWidgetCommand = "fd --type f --hidden --exclude .git";
    changeDirWidgetCommand = "fd --type d --hidden --exclude .git";
  };

  # eza: замена ls (алиасы ls/ll/la/lt/lla)
  programs.eza = {
    enable = true;
    icons = "auto";
    git = true;
  };

  # bat: cat с подсветкой
  programs.bat.enable = true;

  # fd: замена find
  programs.fd.enable = true;
}
