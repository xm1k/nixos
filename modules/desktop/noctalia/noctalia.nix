{ pkgs, inputs, ... }:

{

  home.packages = with pkgs; [
    kdePackages.kdeconnect-kde
    kdePackages.qttools
		evtest
    grim
    slurp
    tesseract
    imagemagick
    zbar
    translate-shell
    wf-recorder
    ffmpeg
    gifski
    jq
    yt-dlp
  ];

	services.cliphist.enable = true;
	services.kdeconnect.enable = true;

  imports = [
    inputs.noctalia.homeModules.default
    ../alacritty/alacritty.nix
    ./hyprlock.nix
  ];

  home.file.".cache/noctalia/wallpapers.json" = {
    text = builtins.toJSON {
      defaultWallpaper = "/etc/nixos/modules/desktop/niri/wallpaper.png";
    };
  };	

  programs.noctalia-shell = {
    enable = true;	

    plugins = {
      sources = [
        {
          enabled = true;
          name    = "Official Noctalia Plugins";
          url     = "https://github.com/noctalia-dev/noctalia-plugins";
        }
      ];

      states = {
        
        "timer" = {
          enabled   = true;
          sourceUrl = "https://github.com/noctalia-dev/noctalia-plugins";
        };

        "todo" = {
          enabled   = true;
          sourceUrl = "https://github.com/noctalia-dev/noctalia-plugins";
        };

        "kde-connect" = {
          enabled   = true;
          sourceUrl = "https://github.com/noctalia-dev/noctalia-plugins";
        };

        "translator" = {
          enabled   = true;
          sourceUrl = "https://github.com/noctalia-dev/noctalia-plugins";
        };

        "battery-threshold" = {
          enabled   = true;
          sourceUrl = "https://github.com/noctalia-dev/noctalia-plugins";
        };

        "screen-recorder" = {
          enabled   = true;
          sourceUrl = "https://github.com/noctalia-dev/noctalia-plugins";
        };	

        "polkit-agent" = {
          enabled   = true;
          sourceUrl = "https://github.com/noctalia-dev/noctalia-plugins";
        };

        "show-keys" = {
          enabled   = false;
          sourceUrl = "https://github.com/noctalia-dev/noctalia-plugins";
        };

        "assistant-panel" = {
          enabled   = true;
          sourceUrl = "https://github.com/noctalia-dev/noctalia-plugins";
        };

        "screen-toolkit" = {
          enabled   = true;
          sourceUrl = "https://github.com/noctalia-dev/noctalia-plugins";
        };

        "custom-sticker" = {
          enabled   = true;
          sourceUrl = "https://github.com/noctalia-dev/noctalia-plugins";
        };

        "music-search" = {
          enabled   = true;
          sourceUrl = "https://github.com/noctalia-dev/noctalia-plugins";
        };

        "keybind-cheatsheet" = {
          enabled   = true;
          sourceUrl = "https://github.com/noctalia-dev/noctalia-plugins";
        };

        "clipper" = {
          enabled   = true;
          sourceUrl = "https://github.com/noctalia-dev/noctalia-plugins";
        };

      };
      
    };

    settings = {	
      general = {
        enableShadows = false;
        iRadiusRatio = 0.44;
        radiusRatio = 0.69;
      };
      colorSchemes = {
        predefinedScheme = "Nord";
      };
      notifications = {
        enableKeyboardLayoutToast = false;
      };
      location = {
        name = "Nizhny Novgorod";
      };
      wallpaper = {
        enabled = true;
        directory = "/etc/nixos/modules/desktop/niri";
        transitionType = [
          "honeycomb"
        ];

      };
      bar = {
        density = "comfortable";
        position = "top";
        widgets = {
          left = [
            {id = "Launcher";}
            {id = "SystemMonitor";}
            {id = "Battery";}
            {id = "plugin:battery-threshold";}
            {id = "Workspace";}
          ];
          center = [
            {id = "plugin:screen-toolkit";}
            {id = "plugin:clipper";}
            {id = "plugin:timer";}
            {id = "plugin:kde-connect";}
            {id = "AudioVisualizer";}
            {id = "plugin:todo";}
            {id = "plugin:assistant-panel";}
          ];
          right = [
            {id = "Volume";}
            {
              id = "KeyboardLayout";
              displayMode = "forceOpen";
              iconColor = "none";
              showIcon = false;
              textColor = "none";
            }
            {id = "Network";}
            {id = "Bluetooth";}
            {id = "Clock";}
            {id = "ControlCenter";}
            {id = "NotificationHistory";}
          ];
        };
      };
    };
    pluginSettings = {
      show-keys = {
        evtestDevice = "/dev/input/event0";
      };
      screen-recorder = {
        videoSource = "screen";
      };
    };
  };
}

