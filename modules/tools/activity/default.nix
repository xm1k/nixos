{ pkgs, ... }:

# Сбор статистики активности (home-manager):
#   shell — atuin пишет каждую команду в SQLite (время, длительность, exit, cwd, сессия).
#           Только запись: Ctrl+R и стрелка вверх остаются как были (fzf / bash).
#   niri  — activity-niri-log (user-сервис) пишет event-stream в ~/.local/share/activity/niri-*.jsonl
#   idle  — hypridle вызывает activity-mark (см. desktop/noctalia/hyprlock.nix), время простоя вычитается
# Отчёт: activity-report [-d дней] [-s shell|niri] [-n топ]
let
  py = "${pkgs.python3}/bin/python3";

  activity-report = pkgs.writeScriptBin "activity-report"
    ("#!${py}\n" + builtins.readFile ./src/report.py);

  activity-niri-log = pkgs.writeScriptBin "activity-niri-log"
    ("#!${py}\n" + builtins.readFile ./src/logger.py);

  # activity-mark <idle|active|lock|unlock|sleep|wake> [сек_назад]
  activity-mark = pkgs.writeShellApplication {
    name = "activity-mark";
    runtimeInputs = [ pkgs.coreutils ];
    text = ''
      dir="''${XDG_DATA_HOME:-$HOME/.local/share}/activity"
      mkdir -p "$dir"
      printf '{"t":%s,"ev":"%s","back":%s}\n' "$(date +%s.%N)" "$1" "''${2:-0}" >> "$dir/idle.jsonl"
    '';
  };
in
{
  home.packages = [ activity-report activity-mark ];

  programs.atuin = {
    enable = true;
    enableBashIntegration = true;
    flags = [ "--disable-up-arrow" "--disable-ctrl-r" ];
    settings = {
      auto_sync = false;
      update_check = false;
      # Не писать в историю секреты, переданные прямо в командной строке
      history_filter = [ "(?i)(password|passwd|token|secret)=\\S+" ];
    };
  };

  # Сам bash-history тоже побольше и с временем — запасной источник
  programs.bash.historySize = 100000;
  programs.bash.historyFileSize = 200000;
  programs.bash.sessionVariables.HISTTIMEFORMAT = "%F %T ";

  systemd.user.services.activity-niri-log = {
    Unit = {
      Description = "Log niri event-stream for activity-report";
      PartOf = [ "graphical-session.target" ];
      After = [ "graphical-session.target" ];
    };
    Service = {
      ExecStart = "${activity-niri-log}/bin/activity-niri-log";
      Environment = [ "ACTIVITY_NIRI=/run/current-system/sw/bin/niri" "PYTHONUNBUFFERED=1" ];
      Restart = "always";
      RestartSec = 5;
    };
    Install.WantedBy = [ "graphical-session.target" ];
  };
}
