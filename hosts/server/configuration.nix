{ config, lib, pkgs, inputs, ... }:

{	

  imports =
    [
      ./hardware-configuration.nix
    ];

  systemd.services.tunnel-manager = {
    description = "Persistent SSH Tunnel";
    after = [ "network-online.target" ];
    wants = [ "network-online.target" ];
    wantedBy = [ "multi-user.target" ];

    serviceConfig = {
      User = "xm1k";
      ExecStart = ''
        ${pkgs.openssh}/bin/ssh -N -T \
          -o StrictHostKeyChecking=no \
          -o ServerAliveInterval=60 \
          -o ExitOnForwardFailure=yes \
          -i /home/xm1k/.ssh/id_ed25519 \
          -p 2213 \
          -R 3000:localhost:3001 \
          -R 8080:localhost:80 \
          -R 8000:localhost:8000 \
          -R 11434:localhost:11434 \
          -R 2223:localhost:22 \
          dasmirnov@194.169.160.248
      '';
      Restart = "always";
      RestartSec = "10";
    };
  };

  networking.hostName = "desktop";

  virtualisation.podman = {
    enable = true;
    dockerCompat = true;
    enableNvidia = true; 
  };

  hardware.nvidia-container-toolkit.enable = true;

}


