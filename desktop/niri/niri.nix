{ config, pkgs, lib, ... }:

{
  environment.systemPackages = with pkgs; [
    alacritty
    fuzzel
    waybar
    mako
  ];

  home-manager.users.root = {

    programs.niri = {
      enable  = true;
      package = pkgs.niri;
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