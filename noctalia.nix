{ pkgs, inputs, ... }:

{
  home-manager.users.root = {
    imports = [
      inputs.noctalia.homeModules.default
    ];

    # включаем Noctalia
    programs.noctalia-shell = {
      enable = true;

      settings = {
        bar.position = "top";
      };
    };

    # (опционально) включить systemd‑сервис через HM
    programs.noctalia-shell.systemd.enable = true;
  };
}