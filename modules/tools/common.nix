{ pkgs, ... }:

{
  home.packages = with pkgs; [
    qbittorrent
    godot_4
    libresprite
  ];
}
