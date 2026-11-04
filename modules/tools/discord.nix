{ pkgs, ... }:

{
  # Блокировки обходит zapret (hosts/default/network.nix), прокси не нужен.
  # Для демонстрации экрана в niri нужен debug.force-pipewire-invalid-modifier
  # (modules/desktop/niri/config.kdl).
  home.packages = [
    (pkgs.discord.override {
      commandLineArgs = "--enable-features=WebRTCPipeWireCapturer";
    })
  ];
}
