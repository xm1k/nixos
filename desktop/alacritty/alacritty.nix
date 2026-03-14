{ pkgs, ... }:

{
	home.packages = with pkgs; [
		alacritty
		maple-mono.NF
		noto-fonts-color-emoji
	];

	fonts.fontconfig = {
		enable = true;
		defaultFonts = {
			monospace = [ 
				"JetBrainsMono Nerd Font" 
				"Noto Color Emoji"
			];
			emoji = ["Noto Color Emoji"];
		};
	};

	home.file.".config/fontconfig/fonts.conf" = {
		source = ./fonts.conf;
		force  = true;
	};

	programs.alacritty = {
		enable = true;
		settings = {
			window = {
				decorations = "None";
				dynamic_padding = true;
				padding = {
					x = 3;
					y = 3;
				};
				opacity = 0.9;
			};

			font = {
				normal = {
					family = "JetBrainsMono Nerd Font";
					style = "Regular";
				};

				size = 11.0;
			};	

		};
	};
}
