{ pkgs, ... }:

{
  programs.nixvim = {
    enable = true;
    defaultEditor = true;

    # Цветовая схема
    colorschemes.catppuccin = {
      enable = true;
      settings.flavour = "mocha";
    };

    # Настройки опций (vim.opt)
    opts = {
      number = true;         # Номера строк
      relativenumber = true; # Относительные номера
      shiftwidth = 2;        # Размер табуляции
      tabstop = 2;
      smartindent = true;
      termguicolors = true;
      cursorline = true;     # Подсветка строки под курсором
    };

    plugins = {
      # Статусная строка
      lualine.enable = true;

      # Дерево файлов
      neo-tree.enable = true;

      # Подсветка синтаксиса
      treesitter.enable = true;

      # Телескоп (поиск всего и вся)
      telescope = {
        enable = true;
        keymaps = {
          "<leader>ff" = "find_files";
          "<leader>fg" = "live_grep";
        };
      };

      # LSP (Умный код)
      lsp = {
        enable = true;
        servers = {
          nil_ls.enable = true;    # Для Nix
          pyright.enable = true;   # Для Python
          ts_ls.enable = true;     # Для JS/TS
        };
      };

      # Автодополнение
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

    # Твои бинды
    keymaps = [
      {
        mode = "n";
        key = "<leader>n";
        action = ":Neotree toggle<CR>";
        options.silent = true;
      }
    ];

    # Глобальная переменная для лидера (пробел)
    globals.mapleader = " ";
  };
}