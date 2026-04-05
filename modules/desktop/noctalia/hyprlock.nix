{ pkgs, ... }:
{

  services.hypridle = {
    enable = true;
    settings = {
      general = {
        lock_cmd = "pidof hyprlock || hyprlock";
        before_sleep_cmd = "loginctl lock-session";
        after_sleep_cmd = "hyprctl dispatch dpms on";
      };

    #   listener = [
    #     {
    #       timeout = 300; # 5 минут
    #       on-timeout = "loginctl lock-session";
    #     }
    #     {
    #       timeout = 600; # 10 минут
    #       on-timeout = "niri msg action power-off-monitors";
    #       on-resume = "niri msg action power-on-monitors";
    #     }
    #   ];
    };
  };

  programs.hyprlock = {
    enable = true;
    settings = {
      general = {
        disable_loading = true;
        grace = 0;
        hide_cursor = true;
      };

      background = [
        {
          path = "/etc/nixos/desktop/niri/wallpaper.png";
          blur_passes = 3;
          blur_size = 8;
          color = "rgb(46, 52, 64)";
        }
      ];

      input-field = [
        {
          size = "250, 60";
          outline_thickness = 2;
          dots_size = 0.2;
          dots_spacing = 0.2;
          dots_center = true;
          outer_color = "rgba(136, 192, 208, 0.5)";
          inner_color = "rgba(46, 52, 64, 0.8)";
          font_color = "rgb(216, 222, 233)";
          fade_on_empty = false;
          placeholder_text = "<i>Password...</i>";
          hide_input = false;
          position = "0, -120";
          halign = "center";
          valign = "center";
        }
      ];

      label = [
        {
          text = "$TIME";
          color = "rgb(236, 239, 244)";
          font_size = 120;
          font_family = "JetBrainsMono Nerd Font";
          position = "0, 80";
          halign = "center";
          valign = "center";
        }
        {
          text = "Hello";
          color = "rgb(143, 188, 187)";
          font_size = 25;
          font_family = "JetBrainsMono Nerd Font";
          position = "0, -40";
          halign = "center";
          valign = "center";
        }
      ];
    };
  };
}
