{ inputs, pkgs, config, ... }:
let
  profileName = "default";
  addons = inputs.nur.legacyPackages.${pkgs.system}.repos.rycee.firefox-addons;
in
{
  programs.firefox = {
    enable = true;

    package = inputs.zen-browser.packages.${pkgs.system}.default;

    profiles.${profileName} = {
      isDefault = true;
      
      extensions = [
        addons.ublock-origin
        addons.darkreader
				# addons.sidebery
				addons.smartproxy
      ];

      settings = {

				"layers.acceleration.force-enabled" = true;
				"gfx.canvas.accelerated" = true;

				"gfx.font_rendering.fontconfig.max_generic_substitutions" = 127;
				"font.name-list.sans-serif.x-western" = "Inter, Roboto, Noto Sans";
				
				"dom.image.lazy_loading.enabled" = true;
				"javascript.options.mem.max_tree_copy_depth" = 1000;
				
				"layout.css.devPixelsPerPx" = "-1.0";
				"browser.display.use_system_colors" = false;


				"media.eme.enabled" = true;
        "media.gmp-widevinecdm.visible" = true;
        "media.gmp-widevinecdm.enabled" = true;
				"browser.toolbars.bookmarks.visibility" = "never";
				"browser.theme.content-theme" = 2;
				"browser.bookmarks.addedImportButton" = false;
        "browser.theme.toolbar-theme" = 2;
        "ui.systemUsesDarkTheme" = 1;
        "extensions.activeThemeID" = "firefox-compact-dark@mozilla.org";
        "intl.accept_languages" = "ru-RU, ru, en-US, en";
        "general.useragent.locale" = "ru-RU";

        "toolkit.legacyUserProfileCustomizations.stylesheets" = true;

				"gfx.webrender.all" = true;
				"gfx.font_rendering.cleartype_params.cleartype_level" = 100;
				"gfx.font_rendering.cleartype_params.rendering_mode" = 5;
				"gfx.font_rendering.directwrite.enabled" = true;
				"layout.css.font-visibility.standard" = 3;

        "browser.aboutConfig.showWarning" = false;
        "browser.shell.checkDefaultBrowser" = false;
        "devtools.chrome.enabled" = true;
        "devtools.debugger.remote-enabled" = true;
      };
    };
  };

  home.file.".zen/profiles.ini".text = ''
    [Profile0]
    Name=${profileName}
    IsRelative=0
    Path=${config.home.homeDirectory}/.mozilla/firefox/${profileName}
    Default=1

    [General]
    StartWithLastProfile=1
    Version=2
  '';

}
