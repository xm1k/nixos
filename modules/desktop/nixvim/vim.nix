{ pkgs, ... }:

{
  home.packages = with pkgs; [
    ripgrep
  ];

  programs.nixvim = {
    enable = true;
    defaultEditor = true;

    clipboard = {
      register = "unnamedplus";
      providers.wl-copy.enable = true;
    }; 

    colorschemes.catppuccin = {
      enable = true;
      settings.flavour = "mocha";
    };

    opts = {
      number = true;
      relativenumber = true;
      shiftwidth = 2;
      tabstop = 2;
			expandtab = true;
      smartindent = true;
      termguicolors = true;
      cursorline = true;
    };	

    plugins = {
      	
			lualine.enable = true;

      neo-tree.enable = true;

      treesitter.enable = true;

      telescope = {
        enable = true;
        keymaps = {
          "<leader>ff" = "find_files";
          "<leader>fg" = "live_grep";
        };
      };

      lsp = {
        enable = true;
        servers = {
          nil_ls.enable = true;
          pyright.enable = true;
          ts_ls.enable = true;
        };
      };

      cmp = {
        enable = true;
        settings = {
          autoEnableSources = true;
          sources = [
            { name = "nvim_lsp"; }
            { name = "path"; }
            { name = "buffer"; }
          ];
          mapping = {
            "<CR>" = "cmp.mapping.confirm({ select = true })";
            "<Tab>" = "cmp.mapping.select_next_item()";
            "<S-Tab>" = "cmp.mapping.select_prev_item()";
          };
        };
      };
    };

    keymaps = [
      {
        mode = "n";
        key = "<leader>e";
        action = ":Neotree focus<CR>";
        options.silent = true;
      }
      {
        mode = "n";
        key = "<leader>n";
        action = ":Neotree toggle<CR>";
        options.silent = true;
      }
    ];

    globals.mapleader = " ";
  };
}

