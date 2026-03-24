{ config, lib, pkgs, inputs, ... }:

{
  
  programs.vscode = {
    enable = true;
    
    extensions = with pkgs.vscode-extensions; [
      continue.continue
    ] 
    ++ (pkgs.nix4vscode.forVscode [
      # "publisher.name" 
    ]);

    userSettings = {
      "editor.fontSize" = 14;
      "workbench.colorTheme" = "Catppuccin Mocha";
    };
  };
}
