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
        # ваши настройки Noctalia здесь
        bar.position = "right";
        # …
      };
    };

    # (опционально) включить systemd‑сервис через HM
    programs.noctalia-shell.systemd.enable = true;
  };
}