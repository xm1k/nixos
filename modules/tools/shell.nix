{ pkgs, ... }:

{
  # zoxide: умный cd (запоминает каталоги). `cd nixos`, `cdi` — интерактивно
  programs.zoxide = {
    enable = true;
    options = [ "--cmd cd" ];
  };

  # fzf: нужен для интерактивного выбора zoxide; заодно Ctrl+R / Ctrl+T / Alt+C
  programs.fzf = {
    enable = true;
    defaultCommand = "fd --type f --hidden --exclude .git";
    fileWidgetCommand = "fd --type f --hidden --exclude .git";
    changeDirWidgetCommand = "fd --type d --hidden --exclude .git";
  };

  # eza: замена ls (алиасы ls/ll/la/lt/lla)
  programs.eza = {
    enable = true;
    icons = "auto";
    git = true;
  };

  # bat: cat с подсветкой
  programs.bat.enable = true;

  # fd: замена find
  programs.fd.enable = true;
}
