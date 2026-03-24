# User tools configuration module
{ config, pkgs, inputs, ... }:

{
  imports = [
    ../../tools/vscode.nix
    ../../tools/work.nix
    ../../tools/podman/podman.nix
  ];
}