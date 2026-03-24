{ config, pkgs, inputs, ... }:

{
  home-manager.useGlobalPkgs = true;
  home-manager.useUserPackages = true;
  home-manager.extraSpecialArgs = { inherit inputs; };

  home-manager.users.xm1k = { pkgs, ... }: {
    home.stateVersion = "25.11";
    
    # imports = [
    #   ../../modules/desktop/firefox/firefox.nix
    #   ../../modules/tools/python.nix
    # ];
  };
}