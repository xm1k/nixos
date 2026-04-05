{ config, lib, pkgs, inputs, ... }:

{
  programs.vscode = {
    enable = true;
    
    extensions = with pkgs.vscode-extensions; [
      continue.continue
    ] ++ (pkgs.nix4vscode.forVscode [
      # "publisher.name" 
    ]);

    userSettings = {
      "editor.fontSize" = 14;
      "workbench.colorTheme" = "Catppuccin Mocha";
      
      # Отключаем скрытое меню
      "window.menuBarVisibility" = "none";
      
      # --- ВОТ ГЛАВНЫЙ ФИКС ---
      # Запрещаем Alt фокусить кастомное меню
      "window.customMenuBarAltFocus" = false; 
      
      # Если после этого всё равно будет моргать, 
      # попробуй поменять "custom" на "native"
      "window.titleBarStyle" = "custom";
      "keyboard.dispatch" = "keyCode";
    };
  };
}
