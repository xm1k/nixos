{ config, pkgs, ... }:

{
  programs.neovim = {
    enable = true;
    defaultEditor = true;
    viAlias = true;
    vimAlias = true;

    # Устанавливаем необходимые плагины
    configure = {
      packages.myVimPackage = with pkgs.vimPlugins; {
        start = [
          # Внешний вид
          catppuccin-nvim
          lualine-nvim
          nvim-web-devicons
          
          # Функционал
          telescope-nvim
          nvim-treesitter.withAllGrammars
          neo-tree-nvim
          
          # LSP и автодополнение
          nvim-lspconfig
          nvim-cmp
          cmp-nvim-lsp
          luasnip
        ];
      };

      # Основные настройки и конфиг плагинов на Lua
      customRC = ''
        lua << EOF
          -- Настройки интерфейса
          vim.opt.number = true         -- Номера строк
          vim.opt.relativenumber = true -- Относительные номера
          vim.opt.shiftwidth = 2        -- Табуляция
          vim.opt.expandtab = true
          vim.opt.termguicolors = true

          -- Тема оформления
          require("catppuccin").setup({ flavour = "mocha" })
          vim.cmd.colorscheme "catppuccin"

          -- Статусная строка
          require('lualine').setup {
            options = { theme = 'catppuccin' }
          }

          -- Настройка Treesitter (подсветка синтаксиса)
          require('nvim-treesitter.configs').setup {
            highlight = { enable = true },
          }

          -- Быстрые клавиши (Keymaps)
          local builtin = require('telescope.builtin')
          vim.keymap.set('n', '<leader>ff', builtin.find_files, {})
          vim.keymap.set('n', '<leader>n', ':Neotree toggle<CR>', {})
        EOF
      '';
    };
  };

  # Дополнительные инструменты для работы (LSP, форматирование)
  environment.systemPackages = with pkgs; [
    nil # LSP для Nix
    lua-language-server
    ripgrep
    fd
  ];
}