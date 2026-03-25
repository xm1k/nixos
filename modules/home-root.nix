# Home Manager configuration for user root
{ config, pkgs, inputs, lib, ... }:

{
  home.stateVersion = "25.11";
  
  imports = [
    ./tools/python.nix
    ./desktop/nixvim/vim.nix
    ./desktop/starship/starship.nix
		./tools/podman/podman.nix
    inputs.nixvim.homeModules.nixvim
  ];
}
