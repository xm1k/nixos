{
  description = "Flake";

  inputs = {

    disko = {
      url = "github:nix-community/disko";
      inputs.nixpkgs.follows = "nixpkgs";
    };

    zen-browser.url = "github:0xc000022070/zen-browser-flake";
    nixpkgs.url = "github:nixos/nixpkgs/nixos-25.11";
		nur.url = "github:nix-community/NUR";

		firefox-theme = {
			url = "git+https://codeberg.org/da157/potatofox.git";
			flake = false;
		};

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
      url = "github:nix-community/nixvim/nixos-25.11";
      inputs.nixpkgs.follows = "nixpkgs";
    };

		nix4vscode = {
      url = "github:nix-community/nix4vscode";
      inputs.nixpkgs.follows = "nixpkgs";
    };

  };

  outputs = { self, nixpkgs, home-manager, noctalia, noctalia-qs, agenix, nixvim, nur, disko, ... }@inputs:

  let
    system = "x86_64-linux";
    pkgs = nixpkgs.legacyPackages.${system};
  in
  {
    nixosConfigurations.nixos = nixpkgs.lib.nixosSystem {
      inherit system;

      specialArgs = { inherit inputs; };

      modules = [
        disko.nixosModules.disko
        ./hosts/nixos/disko-config.nix
        ./hosts/nixos/configuration.nix
        home-manager.nixosModules.home-manager
        agenix.nixosModules.default
        {
          home-manager.useGlobalPkgs = true;
          home-manager.useUserPackages = true;
					home-manager.extraSpecialArgs = { inherit inputs; };
          home-manager.backupFileExtension = "backup";
          home-manager.users.xm1k = import ./modules/home.nix;
          home-manager.users.root = import ./modules/home-root.nix;
        }
      ];
    };
  };
}
