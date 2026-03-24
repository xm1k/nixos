# Home Manager configuration for user xm1k
{ config, pkgs, inputs, ... }:

{
  home.stateVersion = "25.11";
  
  imports = [
    ../desktop/firefox/firefox.nix
    ../tools/python.nix
  ];
}