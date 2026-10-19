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

	services.resolved.enable = false;

  networking = {
    extraHosts = "";
    networkmanager = {
      enable = true;
      dns = "dnsmasq";
    };
  };

  environment.etc."NetworkManager/dnsmasq.d/b-net.conf".text = ''
    address=/b.net/192.168.31.63
    server=77.88.8.8
    server=1.1.1.1
    server=8.8.8.8
    server=/nn.gw.yadro.com/8.8.8.8
    server=/yadro.com/172.31.129.43
  '';

  networking.firewall.enable = false;

  services.tailscale.enable = true;
}
