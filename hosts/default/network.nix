{ pkgs, ... }:

let
	# Порты медиасерверов Discord (голос/стрим, UDP)
	discordUdpPorts = "19294-19344,50000-50100";

	# Не через '' ... '': Nix не срезает табы в начале строк, и hostlist ломается
	discordHosts = pkgs.writeText "zapret-discord-hosts" (builtins.concatStringsSep "\n" [
		"discord.com"
		"discord.gg"
		"discord.co"
		"discord.dev"
		"discord.new"
		"discord.gift"
		"discord.media"
		"discordapp.com"
		"discordapp.net"
		"discordcdn.com"
		"discordstatus.com"
		"dis.gd"
	] + "\n");

	zapretRules = pkgs.writeText "zapret.nft" ''
		table inet zapret {
			chain post {
				type filter hook postrouting priority mangle; policy accept;
				meta mark & 0x40000000 == 0 tcp dport 443 ct original packets 1-6 queue num 200 bypass
				meta mark & 0x40000000 == 0 udp dport { ${discordUdpPorts} } ct original packets 1-12 queue num 200 bypass
			}
		}
	'';
in
{
	services.zapret = {
		enable = true;
		# networking.firewall выключен, правила nftables ставим сами (zapret-nft ниже)
		configureFirewall = false;
		httpSupport = false;
		params = [
			# Голос и демонстрация экрана (WebRTC/DTLS, STUN, протокол Discord)
			"--filter-udp=${discordUdpPorts}"
			"--filter-l7=discord,stun,unknown"
			"--dpi-desync=fake"
			"--dpi-desync-any-protocol=1"
			"--dpi-desync-ttl=3"
			"--dpi-desync-repeats=6"
			"--dpi-desync-cutoff=n4"
			"--new"
			# HTTPS только для доменов Discord
			"--filter-tcp=443"
			"--hostlist=${discordHosts}"
			# Подобрано blockcheck, работает и для TLS1.2, и для TLS1.3
			"--dpi-desync=fake"
			"--dpi-desync-ttl=3"
			"--dpi-desync-fake-tls-mod=rnd,dupsid,rndsni,padencap"
		];
	};

	systemd.services.zapret-nft = {
		description = "nftables rules for zapret";
		wantedBy = [ "multi-user.target" ];
		before = [ "zapret.service" ];
		after = [ "network-pre.target" ];
		serviceConfig = {
			Type = "oneshot";
			RemainAfterExit = true;
			ExecStartPre = "-${pkgs.nftables}/bin/nft delete table inet zapret";
			ExecStart = "${pkgs.nftables}/bin/nft -f ${zapretRules}";
			ExecStop = "${pkgs.nftables}/bin/nft delete table inet zapret";
		};
	};

	# DNS по линкам: NM отдаёт в resolved DNS роутера (DHCP), tailscale регистрирует
	# MagicDNS (100.100.100.100) на tailscale0 через D-Bus, work-vpn вешает yadro.com на tun0.
	services.resolved = {
		enable = true;
		# Используются только если ни у одного интерфейса нет DNS
		fallbackDns = [ "77.88.8.8" "1.1.1.1" ];
	};

	networking.networkmanager = {
		enable = true;
		dns = "systemd-resolved";
	};

  networking.firewall.enable = false;

  services.tailscale.enable = true;
}
