{ pkgs, lib, ... }:

let
  # Блокировки обходит zapret (hosts/default/network.nix), прокси не нужен.
  # Для демонстрации экрана в niri нужен debug.force-pipewire-invalid-modifier
  # (modules/desktop/niri/config.kdl).
  discord = pkgs.discord.override {
    commandLineArgs = "--enable-features=WebRTCPipeWireCapturer";
  };
in
{
  home.packages = [
    discord
    # Запуск без привязки к терминалу (как telegram). hiPrio — чтобы
    # перекрыть bin/discord из самого пакета без коллизии.
    (lib.hiPrio (pkgs.writeShellScriptBin "discord" ''
      setsid ${discord}/bin/discord "$@" > /dev/null 2>&1 &
    ''))
  ];
}
