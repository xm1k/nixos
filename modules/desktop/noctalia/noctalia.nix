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

  programs.noctalia = {
    enable = true;
    package = inputs.noctalia.packages.${pkgs.stdenv.hostPlatform.system}.default;
    settings = {

      audio = {
        enable_power_sounds  = false;
        enable_sounds        = false;
        enable_volume_sounds = false;
      };

      general = {
        enableShadows = false;
        iRadiusRatio  = 0.44;
        radiusRatio   = 0.69;
      };

      colorSchemes.predefinedScheme = "Nord";

      notifications.enableKeyboardLayoutToast = false;

      notification.enable_daemon = false;

      osd.kinds.keyboard_layout = false;

      location.name = "Nizhny Novgorod";

      wallpaper = {
        enabled        = true;
        directory      = "/etc/nixos/modules/desktop/niri";
        transitionType = [ "honeycomb" ];
        default.path   = "/etc/nixos/modules/desktop/niri/wallpaper.png";
      };

      bar = {
        density  = "comfortable";
        position = "top";
        widgets  = {
          capsule        = true;
          font_weight    = 400;
          margin_edge    = 13;
          margin_ends    = 15;
          widget_spacing = 5;
          left = [
            {id = "Launcher";}
            {id = "Battery";}
            {id = "SystemMonitor";}
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
            {id = "Network";}
            {id = "Bluetooth";}
            {id = "Clock";}
            {id = "ControlCenter";}
            {id = "NotificationHistory";}
          ];
        };
      };

      plugins = {
        enabled = [ "noctalia/bongocat" ];
        sources = [
          {
            enabled = true;
            name    = "Official Noctalia Plugins";
            url     = "https://github.com/noctalia-dev/noctalia-plugins";
          }
        ];
        states = {
          "timer"             = { enabled = true;  sourceUrl = "https://github.com/noctalia-dev/noctalia-plugins"; };
          "todo"              = { enabled = true;  sourceUrl = "https://github.com/noctalia-dev/noctalia-plugins"; };
          "kde-connect"       = { enabled = true;  sourceUrl = "https://github.com/noctalia-dev/noctalia-plugins"; };
          "translator"        = { enabled = true;  sourceUrl = "https://github.com/noctalia-dev/noctalia-plugins"; };
          "battery-threshold" = { enabled = true;  sourceUrl = "https://github.com/noctalia-dev/noctalia-plugins"; };
          "polkit-agent"      = { enabled = true;  sourceUrl = "https://github.com/noctalia-dev/noctalia-plugins"; };
          "assistant-panel"   = { enabled = true;  sourceUrl = "https://github.com/noctalia-dev/noctalia-plugins"; };
          "screen-toolkit"    = { enabled = true;  sourceUrl = "https://github.com/noctalia-dev/noctalia-plugins"; };
          "custom-sticker"    = { enabled = true;  sourceUrl = "https://github.com/noctalia-dev/noctalia-plugins"; };
          "music-search"      = { enabled = true;  sourceUrl = "https://github.com/noctalia-dev/noctalia-plugins"; };
          "keybind-cheatsheet"= { enabled = true;  sourceUrl = "https://github.com/noctalia-dev/noctalia-plugins"; };
          "clipper"           = { enabled = true;  sourceUrl = "https://github.com/noctalia-dev/noctalia-plugins"; };
          "screen-recorder"   = {
            enabled   = true;
            sourceUrl = "https://github.com/noctalia-dev/noctalia-plugins";
            settings.videoSource = "screen";
          };
          "show-keys" = {
            enabled   = false;
            sourceUrl = "https://github.com/noctalia-dev/noctalia-plugins";
            settings.evtestDevice = "/dev/input/event0";
          };
        };
      };

      widget = {
        audio_visualizer = { bands = 27; width = 184.0; };
        cat              = { type  = "noctalia/bongocat:cat"; };
        media            = { album_art_only = true; };
        network          = { show_label = false; };
        volume           = { show_label = false; };
      };

      lockscreen_widgets = {
        enabled        = false;
        schema_version = 2;
        widget_order   = [ "lockscreen-login-box@DP-1" "lockscreen-login-box@eDP-1" ];
        grid = {
          cell_size      = 16;
          major_interval = 4;
          visible        = true;
        };
        widget = {
          "lockscreen-login-box@DP-1" = {
            box_height       = 196.0;
            box_width        = 810.0;
            cx               = 960.0;
            cy               = 957.0;
            output           = "DP-1";
            placement_height = 1080.0;
            placement_width  = 1920.0;
            rotation         = 0.0;
            type             = "login_box";
            settings = {
              background_color    = "surface_variant";
              background_opacity  = 0.88;
              background_radius   = 12.0;
              center_password_text= false;
              input_opacity       = 1.0;
              input_radius        = 6.0;
              layout              = "regular";
              show_caps_lock      = true;
              show_keyboard_layout= true;
              show_login_button   = true;
              show_media          = true;
              show_session_buttons= true;
              show_unlock_hint    = true;
              show_weather        = true;
            };
          };
          "lockscreen-login-box@eDP-1" = {
            box_height       = 196.0;
            box_width        = 810.0;
            cx               = 960.0;
            cy               = 1077.0;
            output           = "eDP-1";
            placement_height = 1200.0;
            placement_width  = 1920.0;
            rotation         = 0.0;
            type             = "login_box";
            settings = {
              background_color    = "surface_variant";
              background_opacity  = 0.88;
              background_radius   = 12.0;
              center_password_text= false;
              input_opacity       = 1.0;
              input_radius        = 6.0;
              layout              = "regular";
              show_caps_lock      = true;
              show_keyboard_layout= true;
              show_login_button   = true;
              show_media          = true;
              show_session_buttons= true;
              show_unlock_hint    = true;
              show_weather        = true;
            };
          };
        };
      };

    };
  };
}
