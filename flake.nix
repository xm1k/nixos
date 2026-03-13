{
  description = "My NixOS configuration with Home Manager";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-25.11";
    home-manager = {
      url = "github:nix-community/home-manager/release-25.11";
      inputs.nixpkgs.follows = "nixpkgs";
    };
    noctalia = {
      url = "github:noctalia-dev/noctalia-shell";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.noctalia-qs.follows = "noctalia-qs";
    };
    noctalia-qs = {
      url = "github:noctalia-dev/noctalia-qs";
      inputs.nixpkgs.follows = "nixpkgs";
    };
    agenix = {
      url = "github:ryantm/agenix";
      inputs.nixpkgs.follows = "nixpkgs";
    };
    nixvim = {
      url = "github:nix-community/nixvim";
      inputs.nixpkgs.follows = "nixpkgs";
    };
    
  };

  outputs = { self
            , nixpkgs
            , home-manager
            , noctalia
            , noctalia-qs
            , agenix
            , nixvim
            , ...
            }:

  let
    system = "x86_64-linux";
    pkgs = nixpkgs.legacyPackages.${system};

    inputs = {
      self = self;
      nixpkgs = nixpkgs;
      home-manager = home-manager;
      noctalia = noctalia;
      noctalia-qs = noctalia-qs;
      agenix = agenix;
      nixvim = nixvim;
    };
  in
  {
    nixosConfigurations.nixos = nixpkgs.lib.nixosSystem {
      inherit system;

      specialArgs = { inherit inputs; };

      modules = [
        ./configuration.nix
        home-manager.nixosModules.home-manager
        ./desktop/noctalia/noctalia.nix
        agenix.nixosModules.default
        {
          home-manager.useGlobalPkgs = true;
          home-manager.useUserPackages = true;

          home-manager.users.xm1k = { pkgs, ... }: {
            home.stateVersion = "25.11";
          };
        }
      ];
    };
  };
}