{ config, lib, pkgs, inputs, ... }:

{
  nixpkgs.overlays = [ inputs.nix4vscode.overlays.default ];
  
  programs.vscode = {
    enable = true;
    extensions = (with pkgs.vscode-extensions; [
      continue.continue
    	]) ++ pkgs.nix4vscode.forVscode [
    ];
  };
}
