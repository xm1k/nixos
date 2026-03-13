{ config, pkgs, lib, ... }:

{
  environment.systemPackages = with pkgs; [
    alacritty
    fuzzel
    waybar
    mako
  ];

  programs.niri.enable = true;

  home-manager.users.root = {

    programs.niri = {
      package = niri;
      settings = {
        spawn-at-startup = [
          {
            command = [
              "noctalia-shell"
            ];
          }
        ];
      };
    };

    home.file.".config/niri/config.kdl" = {
      source = ./config.kdl;
      force  = true;
    };
  };

}