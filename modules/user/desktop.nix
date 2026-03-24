# User desktop configuration module
{ config, pkgs, inputs, ... }:

{
  # Import desktop configurations
  imports = [
    ../../desktop/niri/niri.nix
    ../../desktop/nixvim/vim.nix
    ../../desktop/starship/starship.nix
  ];
}
