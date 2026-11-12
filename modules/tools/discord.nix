{ pkgs, lib, ... }:

let
  # Блокировки обходит zapret (hosts/default/network.nix), прокси не нужен.
  # Для демонстрации экрана в niri нужен debug.force-pipewire-invalid-modifier
  # (modules/desktop/niri/config.kdl).
  discord = pkgs.discord.override {
    commandLineArgs = "--enable-features=WebRTCPipeWireCapturer";
  };
  summon = import ../desktop/niri/summon.nix { inherit pkgs; };
in
{
  home.packages = [
    discord
    # Запуск «вместо» терминала: уже открытое окно переносится на место
    # текущего, терминал закрывается (см. niri/summon.nix). hiPrio — чтобы
    # перекрыть bin/discord из самого пакета без коллизии.
    (lib.hiPrio (pkgs.writeShellScriptBin "discord" ''
      exec ${lib.getExe summon} discord ${discord}/bin/discord "$@"
    ''))
  ];
}
