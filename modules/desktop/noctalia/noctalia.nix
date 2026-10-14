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

      theme.builtin = "Nord";

      osd.kinds.keyboard_layout = false;

      notification.enable_daemon = false;

      wallpaper = {
        enabled   = true;
        directory = "/etc/nixos/modules/desktop/niri";
        transition = [ "honeycomb" ];
      };

      bar.widgets = {
        capsule        = true;
        font_weight    = 400;
        margin_edge    = 13;
        margin_ends    = 15;
        widget_spacing = 5;
        start  = [ "launcher" "battery" "cpu" "date" ];
        center = [ "screenshot" "audio_visualizer" "clipboard" ];
        end    = [ "clock" "network" "bluetooth" "volume" "brightness" "media" "control-center" "notifications" ];
      };

      plugins = {
        enabled = [ "noctalia/bongocat" ];
        source  = [
          {
            enabled  = true;
            kind     = "git";
            location = "https://github.com/noctalia-dev/official-plugins";
            name     = "official";
          }
          {
            enabled  = true;
            kind     = "git";
            location = "https://github.com/noctalia-dev/community-plugins";
            name     = "community";
          }
        ];
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
              background_color     = "surface_variant";
              background_opacity   = 0.88;
              background_radius    = 12.0;
              center_password_text = false;
              input_opacity        = 1.0;
              input_radius         = 6.0;
              layout               = "regular";
              show_caps_lock       = true;
              show_keyboard_layout = true;
              show_login_button    = true;
              show_media           = true;
              show_session_buttons = true;
              show_unlock_hint     = true;
              show_weather         = true;
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
              background_color     = "surface_variant";
              background_opacity   = 0.88;
              background_radius    = 12.0;
              center_password_text = false;
              input_opacity        = 1.0;
              input_radius         = 6.0;
              layout               = "regular";
              show_caps_lock       = true;
              show_keyboard_layout = true;
              show_login_button    = true;
              show_media           = true;
              show_session_buttons = true;
              show_unlock_hint     = true;
              show_weather         = true;
            };
          };
        };
      };

    };
  };
}
