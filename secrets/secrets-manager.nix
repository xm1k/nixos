{ config, pkgs, inputs, ... }:

let
  workUser = "da.smirnov";
  # git credential helper для Bitbucket YADRO: берёт WORK_USER/WORK_PASS из env,
  # а если env не подхвачен (GUI/IDE) — читает секрет напрямую.
  bbCredHelper = pkgs.writeShellScript "git-credential-yadro-bb" ''
    [ "$1" = get ] || exit 0
    pass="''${WORK_PASS:-$(cat ${config.age.secrets.workpass.path} 2>/dev/null)}"
    [ -n "$pass" ] || exit 0
    echo "username=''${WORK_USER:-${workUser}}"
    echo "password=$pass"
  '';
in
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
    "gitlab_token" = {
			file = ./gitlab_token.age;
			owner = "xm1k";
			group = "users";
			mode = "0400";
		};
		"tailscale_authkey" = {
			file = ./tailscale_authkey.age;
			owner = "xm1k";
			group = "users";
			mode = "0400";
		};
		"work_env" = {
			file = ./work_env.age;
			owner = "xm1k";
			group = "users";
			mode = "0400";
		};
  };

  # Системный /etc/gitconfig: для b.yadro.com логин/пароль подставляются автоматически.
  programs.git = {
    enable = true;
    config.credential."https://b.yadro.com" = {
      helper = "${bbCredHelper}";
      username = workUser;
    };
  };

  environment.systemPackages = [
    inputs.agenix.packages."${pkgs.system}".default
  ];

	environment.extraInit = "
		export FREE_PORT_PASS=$(cat ${config.age.secrets.freeportpass.path} 2>/dev/null)
		export GH_TOKEN=$(cat ${config.age.secrets.github_token.path} 2>/dev/null)
		export WORK_PASS=$(cat ${config.age.secrets.workpass.path} 2>/dev/null)
		export WORK_USER=${workUser}
		export SSH_PASS=$(cat ${config.age.secrets.password.path} 2>/dev/null)
		export GITLAB_TOKEN=$(cat ${config.age.secrets.gitlab_token.path} 2>/dev/null)
		export TS_AUTHKEY=$(cat ${config.age.secrets.tailscale_authkey.path} 2>/dev/null)
		export DEPLOY_USER=xm1k
		# work_env (KEY=VALUE, без кавычек): построчно, без eval — значения с $ ! * не раскрываются.
		if [ -r ${config.age.secrets.work_env.path} ]; then
			while IFS= read -r __l || [ -n \"$__l\" ]; do
				case \"$__l\" in ''|'#'*) continue ;; esac
				export \"\${__l%%=*}=\${__l#*=}\"
			done < ${config.age.secrets.work_env.path}
			unset __l
		fi
	";
}
