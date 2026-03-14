{ config, pkgs, inputs, ... }:

{
  imports = [ inputs.agenix.nixosModules.default ];

  age.identityPaths = [
    "/home/xm1k/.ssh/id_ed25519"
  ];

  age.secrets = {
    "password" = {
      file = ./password.age;
      owner = "xm1k";
      group = "users";
      mode = "0400";
    };
		"github_token" = {
      file = ./github_token.age;
      owner = "xm1k";
      group = "users";
      mode = "0400";
    };
		"freeportpass" = {
			file = ./freeportpass.age;
			owner = "xm1k";
			group = "users";
			mode = "0400";
		};
		"workpass" = {
			file = ./workpass.age;
			owner = "xm1k";
			group = "users";
			mode = "0400";
		};
  };

  environment.systemPackages = [
    inputs.agenix.packages."${pkgs.system}".default
  ];

	environment.extraInit = "
		export FREE_PORT_PASS=$(cat ${config.age.secrets.freeportpass.path} 2>/dev/null)
		export GH_TOKEN=$(cat ${config.age.secrets.github_token.path} 2>/dev/null)	
		export WORK_PASS=$(cat ${config.age.secrets.workpass.path} 2>/dev/null)	
	";
}
