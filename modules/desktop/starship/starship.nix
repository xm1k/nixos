{ pkgs, lib, ... }:

{
  programs.starship = {
    enable = true;
    settings = builtins.fromTOML (builtins.readFile ./starship.toml);
  };

  programs.bash = {
    enable = true;
  };

}