{ pkgs }:

# niri-summon <app-id> <command...>
#
# Запуск GUI-приложения «вместо» терминала:
#  - окно с таким app-id уже есть → переносим его на место текущего окна
#    (тот же монитор/воркспейс/колонка) и закрываем терминал;
#  - окна нет → запускаем приложение (оно откроется рядом с терминалом),
#    дожидаемся окна и закрываем терминал.
# Терминал закрывается, только если скрипт действительно запущен из
# сфокусированного окна (его PID — предок скрипта); иначе просто
# фокусируем/запускаем приложение.
pkgs.writeShellApplication {
  name = "niri-summon";
  runtimeInputs = [ pkgs.jq pkgs.coreutils pkgs.util-linux ];
  text = ''
    # Фоновый режим: ждём окно приложения и закрываем терминал.
    if [ "''${1:-}" = "--close-when-open" ]; then
      app_id="$2"; term_id="$3"
      for _ in $(seq 300); do
        if niri msg -j windows | jq -e --arg a "$app_id" 'any(.[]; .app_id == $a)' >/dev/null; then
          niri msg action close-window --id "$term_id"
          exit 0
        fi
        sleep 0.1
      done
      exit 0
    fi

    app_id="$1"; shift

    # Сфокусированное окно — это наш терминал?
    term_id=""; term_ws=""
    focused=$(niri msg -j focused-window 2>/dev/null || echo null)
    fpid=$(jq -r '.pid // empty' <<<"$focused")
    if [ -n "$fpid" ]; then
      p=$$
      while [ "$p" -gt 1 ]; do
        if [ "$p" = "$fpid" ]; then
          term_id=$(jq -r '.id' <<<"$focused")
          term_ws=$(jq -r '.workspace_id' <<<"$focused")
          break
        fi
        stat=$(< "/proc/$p/stat") || break
        read -r _ p _ <<<"''${stat##*) }"
      done
    fi

    # Уже открытое окно (самое недавно сфокусированное).
    win=$(niri msg -j windows | jq -c --arg a "$app_id" '
      [.[] | select(.app_id == $a)]
      | max_by((.focus_timestamp.secs // 0) * 1000000000 + (.focus_timestamp.nanos // 0))
      // empty')

    if [ -z "$win" ]; then
      setsid "$@" >/dev/null 2>&1 </dev/null &
      if [ -n "$term_id" ]; then
        setsid "$0" --close-when-open "$app_id" "$term_id" >/dev/null 2>&1 </dev/null &
      fi
      exit 0
    fi

    wid=$(jq -r '.id' <<<"$win")
    if [ -z "$term_id" ]; then
      niri msg action focus-window --id "$wid"
      exit 0
    fi

    wws=$(jq -r '.workspace_id' <<<"$win")
    if [ "$wws" != "$term_ws" ]; then
      ws=$(niri msg -j workspaces)
      term_out=$(jq -r --argjson w "$term_ws" '.[] | select(.id == $w) | .output' <<<"$ws")
      win_out=$(jq -r --argjson w "$wws" '.[] | select(.id == $w) | .output' <<<"$ws")
      if [ "$win_out" != "$term_out" ]; then
        niri msg action move-window-to-monitor --id "$wid" "$term_out"
      fi
      # Индекс берём заново: воркспейсы могли перенумероваться.
      idx=$(niri msg -j workspaces | jq -r --argjson w "$term_ws" '.[] | select(.id == $w) | .idx')
      niri msg action move-window-to-workspace --window-id "$wid" --focus false "$idx"
    fi

    niri msg action focus-window --id "$wid"
    col=$(niri msg -j windows | jq -r --argjson t "$term_id" \
      '.[] | select(.id == $t) | .layout.pos_in_scrolling_layout[0] // empty')
    if [ -n "$col" ]; then
      niri msg action move-column-to-index "$col"
    fi
    niri msg action close-window --id "$term_id"
  '';
}
