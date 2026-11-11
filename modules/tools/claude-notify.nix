# Уведомления Claude Code: «закончил» / «ждёт ответа» с указанием терминала.
# Клик по кнопке уведомления фокусирует нужное окно Alacritty в niri.
# Уведомление не показывается, если окно с этим claude уже в фокусе.
{ pkgs, lib, ... }:

let
  claude-notify = pkgs.writeShellApplication {
    name = "claude-notify";
    runtimeInputs = with pkgs; [ jq libnotify niri procps coreutils ];
    text = ''
      event="''${1:-stop}"
      input="$(cat)"

      cwd="$(jq -r '.cwd // empty' <<<"$input")"
      project="$(basename "''${cwd:-$PWD}")"

      # Ищем окно niri, которому принадлежит этот claude: идём вверх по PPID,
      # пока pid не совпадёт с pid какого-либо окна.
      windows="$(niri msg --json windows 2>/dev/null || echo '[]')"
      win_id="" ; win_title=""
      pid=$$
      while [ -n "$pid" ] && [ "$pid" -gt 1 ]; do
        match="$(jq -c --argjson p "$pid" 'map(select(.pid == $p)) | first // empty' <<<"$windows")"
        if [ -n "$match" ]; then
          win_id="$(jq -r '.id' <<<"$match")"
          win_title="$(jq -r '.title' <<<"$match")"
          break
        fi
        pid="$(ps -o ppid= -p "$pid" | tr -d ' ')"
      done

      # Окно уже в фокусе — не мешаем.
      focused="$(niri msg --json focused-window 2>/dev/null | jq -r '.id // empty')"
      if [ -n "$win_id" ] && [ "$win_id" = "$focused" ]; then
        exit 0
      fi

      case "$event" in
        notification)
          summary="Claude ждёт ответа · $project"
          body="$(jq -r '.message // "Требуется внимание"' <<<"$input")"
          urgency=critical
          ;;
        *)
          summary="Claude закончил · $project"
          body="''${win_title:-$cwd}"
          urgency=normal
          ;;
      esac

      # В фоне, чтобы хук сразу вернул управление claude.
      (
        action="$(notify-send -a "Claude Code" -u "$urgency" -i utilities-terminal \
          -A "focus=Перейти" "$summary" "$body" 2>/dev/null || true)"
        if [ "$action" = "focus" ] && [ -n "$win_id" ]; then
          niri msg action focus-window --id "$win_id"
        fi
      ) </dev/null >/dev/null 2>&1 &
      disown
    '';
  };

  hooks = {
    hooks = {
      Stop = [{ hooks = [{ type = "command"; command = "${claude-notify}/bin/claude-notify stop"; }]; }];
      Notification = [{ hooks = [{ type = "command"; command = "${claude-notify}/bin/claude-notify notification"; }]; }];
    };
  };
  hooksJson = pkgs.writeText "claude-hooks.json" (builtins.toJSON hooks);
in
{
  home.packages = [ claude-notify pkgs.libnotify ];

  # ~/.claude/settings.json остаётся обычным изменяемым файлом (claude сам пишет в него),
  # поэтому не делаем симлинк, а вливаем секцию hooks через jq при активации.
  home.activation.claudeNotifyHooks = lib.hm.dag.entryAfter [ "writeBoundary" ] ''
    f="$HOME/.claude/settings.json"
    mkdir -p "$HOME/.claude"
    [ -s "$f" ] || echo '{}' > "$f"
    tmp="$(mktemp)"
    ${pkgs.jq}/bin/jq -s '.[0] * .[1]' "$f" ${hooksJson} > "$tmp" && mv "$tmp" "$f"
  '';
}
