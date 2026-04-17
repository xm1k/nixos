# Edit this configuration file to define what should be installed on
# your system. Help is available in the configuration.nix(5) man page, on
# https://search.nixos.org/options and in the NixOS manual (`nixos-help`).

{ config, lib, pkgs, inputs, ... }:

{	

  imports =
    [ # Include the results of the hardware scan.
      ./network.nix
      ./screen.nix
      ./games.nix
			../../secrets/secrets-manager.nix
    ];



  environment.systemPackages = with pkgs; [
    podman-compose
    wine
    vivaldi
    wget
    curl
    git
    python310
    docker
		wl-clipboard
		unzip
		unrar
		mpv
		ansible
		byedpi
		obsidian
		k9s
    kubectl
    kubernetes-helm
    cilium-cli
    gum
		dnsutils
		nautilus
		steam-run
		brightnessctl
  	bottom
		prismlauncher
    onlyoffice-desktopeditors

    neo # neo-matrix
    hollywood
    lavat
    peaclock
    ticker
    mapscii
    tenki
    cbonsai
    pipes-rs
		fastfetch
    hyfetch
    onefetch
    cpufetch
    cava
    btop
    gtop
    cool-retro-term
    toilet
    figlet
    lolcat

    # work
    openldap
    apache-directory-studio
	];

	

	security.pki.certificateFiles = [ 
    "${pkgs.cacert}/etc/ssl/certs/ca-bundle.crt"
    ./certs/work.crt
  ];

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
  
    # Use the systemd-boot EFI boot loader.
  nixpkgs.config.allowUnfree = true;
  boot.loader.systemd-boot.enable = true;
  boot.loader.efi.canTouchEfiVariables = true;

  # networking.hostName = "nixos"; # Define your hostname.

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

  environment.variables = {
		DISPLAY = ":0";
  };

  nixpkgs.overlays = [ inputs.nix4vscode.overlays.default ];

  virtualisation = {
    containers.enable = true;

    containers.containersConf.settings = {
      containers = {
        log_driver = "journald";
      };
    };

		podman = {
      enable = true;
		  dockerCompat = true;
    };
	};

  services.openssh.enable = true;
  services.openssh.settings = {
    PermitRootLogin = "yes";
    PasswordAuthentication = true;
  };

  programs.gamemode.enable = true;	

	services.pipewire = {
		enable = true;
		alsa.enable = true;
		alsa.support32Bit = true;
		pulse.enable = true;
		jack.enable = true;
		wireplumber.enable = true;
	};	

  system.stateVersion = "25.11"; # Did you read the comment?

}


