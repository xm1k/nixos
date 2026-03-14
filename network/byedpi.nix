{ pkgs, ... }:

{
	systemd.services.byedpi = {
		description = "ByeDPI Proxy Service";
		
		after = [ "network.target" ];
		wantedBy = [ "multi-user.target" ];

		serviceConfig = {
			ExecStart = "${pkgs.byedpi}/bin/ciadpi --ip 127.0.0.1 --port 1080 -s1 -o1 -Ar -At -f-1 -r1+s -As";
			
			Restart = "always";
			RestartSec = "5s";

			User = "xm1k";
			
			ProtectSystem = "full";
			NoNewPrivileges = true;
		};
	};
}
