# Edit this configuration file to define what should be installed on
# your system. Help is available in the configuration.nix(5) man page, on
# https://search.nixos.org/options and in the NixOS manual (`nixos-help`).

{ config, lib, pkgs, inputs, ... }:

{	

	security.pki.certificateFiles = [ "${pkgs.cacert}/etc/ssl/certs/ca-bundle.crt" ];

	environment.variables = {
			SSL_CERT_FILE = "${pkgs.cacert}/etc/ssl/certs/ca-bundle.crt";
			SSL_CERT_DIR = "${pkgs.cacert}/etc/ssl/certs";
			NIX_SSL_CERT_FILE = "${pkgs.cacert}/etc/ssl/certs/ca-bundle.crt";
			NIXOS_OZONE_WL = "1";
	};

	fonts.packages = with pkgs; [
    nerd-fonts.jetbrains-mono
    nerd-fonts.fira-code
  ];
  
  imports =
    [ # Include the results of the hardware scan.
      ./games/core.nix
      ./hardware-configuration.nix
      ./desktop/niri/niri.nix
      ./desktop/nixvim/vim.nix
			./tools/vscode.nix
			./tools/work.nix
      ./desktop/starship/starship.nix
			./tools/podman/podman.nix
			./secrets/secrets-manager.nix
			./network/byedpi.nix
    ];

  # Use the systemd-boot EFI boot loader.
  nixpkgs.config.allowUnfree = true;
  boot.loader.systemd-boot.enable = true;
  boot.loader.efi.canTouchEfiVariables = true;

  # networking.hostName = "nixos"; # Define your hostname.

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

  # Set your time zone.
  time.timeZone = "Europe/Moscow";

  # Configure network proxy if necessary
  # networking.proxy.default = "http://user:password@proxy:port/";
  # networking.proxy.noProxy = "127.0.0.1,localhost,internal.domain";

  # Select internationalisation properties.
  # i18n.defaultLocale = "en_US.UTF-8";
  # console = {
  #   font = "Lat2-Terminus16";
  #   keyMap = "us";
  #   useXkbConfig = true; # use xkb.options in tty.
  # };

  # Enable the X11 windowing system.
  # services.xserver.enable = true;

	# RAM managing
	boot.tmp.useTmpfs = true;
	boot.tmp.tmpfsSize = "50%"; # выделяем до половины ОЗУ под /tmp
	services.psd.enable = true;

  nix.settings = {
    show-trace = true;
    extra-experimental-features = [ "nix-command" "flakes" ];
    extra-sandbox-paths = [];
    max-jobs = 4;
  };


  # Configure keymap in X11
  # services.xserver.xkb.layout = "us";
  # services.xserver.xkb.options = "eurosign:e,caps:escape";

  # Enable CUPS to print documents.
  # services.printing.enable = true;

  # Enable sound.
  # services.pulseaudio.enable = true;
  # OR
  # services.pipewire = {
  #   enable = true;
  #   pulse.enable = true;
  # };

  # Enable touchpad support (enabled default in most desktopManager).
  # services.libinput.enable = true;

  # Define a user account. Don't forget to set a password with ‘passwd’.
  users.groups.battery_ctl = {};

	users.users.xm1k = {
    isNormalUser = true;
    extraGroups = [ "wheel" "networkmanager" "video" "battery_ctl" "input" ];
    initialPassword = "password";
  };

  services.xserver = {
    enable = true;
    xkb = {
      layout = "us,ru";
      variant = ",";
      options = "grp:alt_shift_toggle";
    };
  };
  services.displayManager.sddm.enable = true;
  services.displayManager.autoLogin.enable = true;
  services.displayManager.autoLogin.user = "xm1k";
  services.displayManager.defaultSession = "niri";

  services.getty.autologinUser = "xm1k";
	services.upower.enable = true;

  programs.niri.enable = true;

  # List packages installed in system profile.
  # You can use https://search.nixos.org/ to find more packages (and options).
  environment.systemPackages = with pkgs; [
    vivaldi
    wget
    curl
    git
    python310
    docker
		wl-clipboard
		fastfetch
		unzip
		unrar
		mpv
		ansible
		byedpi
		obsidian
		k9s
		dnsutils
		nautilus
		steam-run
		brightnessctl
  	bottom
		prismlauncher
    onlyoffice-desktopeditors
	];

	services.udev.extraRules = ''
    SUBSYSTEM=="power_supply", KERNEL=="BAT*", \
      RUN+="${pkgs.coreutils}/bin/chgrp battery_ctl /sys%p/charge_control_end_threshold", \
      RUN+="${pkgs.coreutils}/bin/chmod g+w /sys%p/charge_control_end_threshold"
  '';	

  environment.variables = {
		DISPLAY = ":0";
  };

  # Some programs need SUID wrappers, can be configured further or are
  # started in user sessions.
  # programs.mtr.enable = true;
  # programs.gnupg.agent = {
  #   enable = true;
  #   enableSSHSupport = true;
  # };

  # List services that you want to enable:

  # Enable the OpenSSH daemon.
  services.openssh.enable = true;
  services.openssh.settings = {
    PermitRootLogin = "yes";
    PasswordAuthentication = true;
  };

  # Open ports in the firewall.
  # networking.firewall.allowedTCPPorts = [ ... ];
  # networking.firewall.allowedUDPPorts = [ ... ];
  # Or disable the firewall altogether.
  networking.firewall.enable = false;

	hardware = {
		bluetooth = {
			enable = true;
			powerOnBoot = true;
			settings = {
				General = {
					Enable = "Source,Sinc,Media,Socket";
					Experimental = true;
				};
			};
		};
		graphics = {
			enable = true;
			extraPackages = with pkgs; [
				intel-media-driver
				libvdpau-va-gl
			];
		};
	};

	services.pipewire = {
		enable = true;
		alsa.enable = true;
		alsa.support32Bit = true;
		pulse.enable = true;
		jack.enable = true;
		wireplumber.enable = true;
	};
	services.thermald.enable = true;

	services.tlp = {
		enable = true;
		settings = {
			GPU_SCALING_GOVERNOR_ON_AC = "performance";
			GPU_SCALING_GOVERNOR_ON_BAT = "powersave";

			PCIE_ASPM_ON_BAT = "powersupersave";
			INTEL_GPU_MIN_FREQ_ON_BAT = 300;
			INTEL_GPU_MAX_FREQ_ON_BAT = 800;
			INTEL_GPU_BOOST_ON_BAT = 0;

			CPU_ENERGY_PERF_POLICY_ON_AC = "performance";
			CPU_ENERGY_PERF_POLICY_ON_BAT = "power";

			CPU_BOOST_ON_AC = 1;
			CPU_BOOST_ON_BAT = 0;
		};
	};

	services.power-profiles-daemon.enable = false;

  system.stateVersion = "25.11"; # Did you read the comment?

}


