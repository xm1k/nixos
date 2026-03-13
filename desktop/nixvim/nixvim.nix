programs.nixvim = {
  enable = true;

  colorschemes.catppuccin.enable = true;

  plugins = {
    lualine.enable = true;
    telescope.enable = true;
    treesitter.enable = true;
    nvim-tree.enable = true;

    cmp = {
      enable = true;
      autoEnableSources = true;
    };

    lsp.enable = true;
  };

  opts = {
    number = true;
    relativenumber = true;
  };
};
