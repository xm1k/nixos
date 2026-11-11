# Уведомление «Claude закончил»: персонаж (claude-peek) выглядывает из-за
# нижнего края экрана и в облачке пишет, какая сессия закончила работу.
# ЛКМ по нему фокусирует нужное окно Alacritty в niri, ПКМ — закрыть.
# Не показывается, если окно с этим claude уже в фокусе.
#
# Показ идёт через службу systemd --user `claude-peek`:
#   claude-peek-off / claude-peek-on — выключить/включить (до перезагрузки),
#   во время демонстрации экрана (niri msg casts) служба сама молчит.
{ pkgs, lib, ... }:

let
  peekPython = pkgs.python3.withPackages (ps: [ ps.pygobject3 ps.pycairo ]);

  # Пиксельный шрифт с кириллицей — только для claude-peek.
  peekFonts = pkgs.makeFontsConf { fontDirectories = [ pkgs.terminus_font_ttf pkgs.dejavu_fonts ]; };

  claude-peek = pkgs.stdenv.mkDerivation {
    pname = "claude-peek";
    version = "1";
    dontUnpack = true;
    nativeBuildInputs = with pkgs; [ wrapGAppsHook3 gobject-introspection makeWrapper ];
    buildInputs = with pkgs; [ gtk3 gtk-layer-shell pango ];
    dontWrapGApps = true;
    installPhase = ''
      install -Dm644 ${./peek.py} $out/share/claude-peek/peek.py
      mkdir -p $out/bin
    '';
    postFixup = ''
      makeWrapper ${peekPython}/bin/python3 $out/bin/claude-peek \
        "''${gappsWrapperArgs[@]}" \
        --prefix PATH : ${lib.makeBinPath [ pkgs.niri ]} \
        --set FONTCONFIG_FILE ${peekFonts} \
        --add-flags $out/share/claude-peek/peek.py
    '';
  };

  claude-notify = pkgs.writeShellApplication {
    name = "claude-notify";
    runtimeInputs = [ claude-peek ] ++ (with pkgs; [ jq niri procps coreutils ]);
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

      [ "$event" = stop ] || exit 0

      # Служба выключена — сокета нет, claude-peek --notify тихо ничего не делает.
      claude-peek --notify --project "$project" --detail "''${win_title:-$cwd}" --win-id "$win_id" \
        </dev/null >/dev/null 2>&1 &
      disown
    '';
  };

  hooks = {
    hooks = {
      Stop = [{ hooks = [{ type = "command"; command = "${claude-notify}/bin/claude-notify stop"; }]; }];
    };
  };
  hooksJson = pkgs.writeText "claude-hooks.json" (builtins.toJSON hooks);

  peekOn = pkgs.writeShellScriptBin "claude-peek-on" ''
    systemctl --user start claude-peek.service && echo "claude-peek: включён"
  '';
  peekOff = pkgs.writeShellScriptBin "claude-peek-off" ''
    systemctl --user stop claude-peek.service && echo "claude-peek: выключен до перезагрузки (claude-peek-on — вернуть)"
  '';
in
{
  home.packages = [ claude-notify claude-peek peekOn peekOff ];

  systemd.user.services.claude-peek = {
    Unit = {
      Description = "Claude Code: пиксельное уведомление о завершении сессии";
      PartOf = [ "graphical-session.target" ];
      After = [ "graphical-session.target" ];
    };
    Service = {
      ExecStart = "${claude-peek}/bin/claude-peek --daemon";
      Restart = "on-failure";
      RestartSec = 2;
    };
    Install.WantedBy = [ "graphical-session.target" ];
  };

  # ~/.claude/settings.json остаётся обычным изменяемым файлом (claude сам пишет в него),
  # поэтому не делаем симлинк, а вливаем секцию hooks через jq при активации.
  home.activation.claudeNotifyHooks = lib.hm.dag.entryAfter [ "writeBoundary" ] ''
    f="$HOME/.claude/settings.json"
    mkdir -p "$HOME/.claude"
    [ -s "$f" ] || echo '{}' > "$f"
    tmp="$(mktemp)"
    # Заодно выкидываем старый хук Notification от claude-notify («ждёт ответа»).
    ${pkgs.jq}/bin/jq -s '
      (.[0] * .[1])
      | if .hooks.Notification then
          .hooks.Notification |= map(select([.hooks[]?.command // ""] | any(test("claude-notify")) | not))
          | if .hooks.Notification == [] then del(.hooks.Notification) else . end
        else . end
    ' "$f" ${hooksJson} > "$tmp" && mv "$tmp" "$f"
  '';
}
