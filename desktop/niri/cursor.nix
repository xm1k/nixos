{ pkgs, inputs, ... }:

{
	home.pointerCursor = {
    gtk.enable = true;
    x11.enable = true;
    package = pkgs.bibata-cursors;
    name = "Bibata-Modern-Ice";
    size = 24;
  };

	gtk = {
    enable = true;
    theme = {
      name = "Noctalia";
      package = inputs.noctalia-qs.packages.${pkgs.system}.default;
    };
  };
}
