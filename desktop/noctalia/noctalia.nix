{ pkgs, inputs, ... }:

{
  home-manager.users.root = {
    imports = [
      inputs.noctalia.homeModules.default
    ];

    programs.noctalia-shell = {
      enable = true;

      settings = {
        bar.position = "top";
      };
    };
  };
}