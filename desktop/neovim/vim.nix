{
  programs.neovim = {
    enable = true;
    viAlias = true;
    vimAlias = true;

    configure = {
      customRC = ''
        set number
        set relativenumber
        set tabstop=4
        set shiftwidth=4
        set expandtab
        set smartindent
        set mouse=a
        set termguicolors
        set clipboard=unnamedplus

        colorscheme gruvbox

        " keybinds
        nnoremap <leader>e :NvimTreeToggle<CR>
        nnoremap <leader>f :Telescope find_files<CR>
        nnoremap <leader>g :Telescope live_grep<CR>
      '';

      packages.myVimPackage = with pkgs.vimPlugins; {
        start = [
          gruvbox

          # icons
          nvim-web-devicons

          # file explorer
          nvim-tree-lua

          # fuzzy search
          telescope-nvim
          plenary-nvim

          # syntax highlighting
          (nvim-treesitter.withAllGrammars)

          # status line
          lualine-nvim

          # git
          gitsigns-nvim
        ];
      };
    };
  };
}