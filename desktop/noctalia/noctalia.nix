{ pkgs, inputs, ... }:

{
  environment.systemPackages = with pkgs; [
    pkgs.kdePackages.kdeconnect
    pkgs.kdePackages.qttools
  ];

  home-manager.users.root = {
    imports = [
      inputs.noctalia.homeModules.default
    ];

    programs.noctalia-shell = {
      enable = true;

      plugins = {
        sources = [
          {
            enabled = true;
            name    = "Official Noctalia Plugins";
            url     = "https://github.com/noctalia-dev/noctalia-plugins";
          }
        ];

        states = {
          "kde-connect" = {
            enabled   = true;
            sourceUrl = "https://github.com/noctalia-dev/noctalia-plugins";
          };
        };

        version = 2;
      };

      settings = {
        bar.position = "top";
      };
    };
  };
}