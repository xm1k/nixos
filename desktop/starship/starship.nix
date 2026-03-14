{ pkgs, ... }:

{
  environment.systemPackages = with pkgs; [
    starship
  ];

  programs.bash.promptInit = ''
    eval "$(starship init bash)"
  '';

	environment.variables.STARSHIP_CONFIG = "/etc/starship.toml";
  environment.etc."starship.toml".source = ./starship.toml;

	home-manager.users.xm1k = {
    home.file.".config/starship.toml" = {
      source = ./starship.toml;
      force  = true;
    };
  };

}


