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

	services.resolved = {
		enable = true;
		fallbackDns = [ "1.1.1.1" "8.8.8.8" ];
	};

  networking= {
    extraHosts = ''
      192.168.31.63 b.net
    '';
		networkmanager = {
			enable = true;
			dns = "systemd-resolved";
		};
		nameservers = [ "1.1.1.1" "8.8.8.8" ];
	};
 
  networking.firewall.enable = false;
}
