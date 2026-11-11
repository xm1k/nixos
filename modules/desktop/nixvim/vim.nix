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
      relativenumber = false;
      shiftwidth = 2;
      tabstop = 2;
			expandtab = true;
      smartindent = true;
      termguicolors = true;
      cursorline = true;
      # diff: всегда вертикально, более точный алгоритм и выравнивание строк
      diffopt = "internal,filler,closeoff,vertical,algorithm:histogram,indent-heuristic,linematch:60";
    };	

    plugins = {

      trouble.enable = true;

			lualine.enable = true;

      neo-tree = {
        enable = true;
        # освобождаем <C-f>, чтобы глобальный поиск файлов работал и в дереве
        settings.window.mappings."<C-f>" = "none";
      };

      treesitter.enable = true;

      # Поиск и замена как в VS Code (Ctrl+H / Ctrl+Shift+H)
      grug-far.enable = true;

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
          pyright.enable = false;
          jedi_language_server.enable = true;
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
      # --- Diff двух файлов ---
      # Выбрать файл через Telescope и открыть его справа в режиме diff с текущим
      {
        mode = "n";
        key = "<leader>dd";
        action.__raw = ''
          function()
            local actions = require("telescope.actions")
            local action_state = require("telescope.actions.state")
            require("telescope.builtin").find_files({
              prompt_title = "Diff with",
              attach_mappings = function(prompt_bufnr)
                actions.select_default:replace(function()
                  local entry = action_state.get_selected_entry()
                  actions.close(prompt_bufnr)
                  if entry then
                    local path = entry.path or entry.filename or entry[1]
                    vim.cmd("vert diffsplit " .. vim.fn.fnameescape(path))
                  end
                end)
                return true
              end,
            })
          end
        '';
        options.silent = true;
        options.desc = "Diff current file with another file";
      }
      # Включить/выключить diff для уже открытых рядом окон
      {
        mode = "n";
        key = "<leader>dw";
        action.__raw = ''
          function()
            if vim.wo.diff then
              vim.cmd("diffoff!")
            else
              local win = vim.api.nvim_get_current_win()
              vim.cmd("windo if &buftype ==# ''' | diffthis | endif")
              vim.api.nvim_set_current_win(win)
            end
          end
        '';
        options.silent = true;
        options.desc = "Toggle diff for visible windows";
      }
      # Выйти из diff: выключить во всех окнах и закрыть остальные
      {
        mode = "n";
        key = "<leader>dq";
        action = "<cmd>diffoff! | only<CR>";
        options.silent = true;
        options.desc = "Close diff";
      }
      {
        mode = "n";
        key = "<C-f>";
        action = "<cmd>Telescope find_files<CR>";
        options.silent = true;
        options.desc = "Find files by name";
      }
      {
        mode = "n";
        key = "<C-S-f>";
        action = "<cmd>Telescope live_grep<CR>";
        options.silent = true;
        options.desc = "Search text in files";
      }
      {
        mode = "n";
        key = "<C-h>";
        action.__raw = ''function() require("grug-far").open({ prefills = { paths = vim.fn.expand("%") } }) end'';
        options.silent = true;
        options.desc = "Replace in current file";
      }
      {
        mode = "v";
        key = "<C-h>";
        action.__raw = ''function() require("grug-far").with_visual_selection({ prefills = { paths = vim.fn.expand("%") } }) end'';
        options.silent = true;
        options.desc = "Replace selection in current file";
      }
      {
        mode = "n";
        key = "<C-S-h>";
        action.__raw = ''function() require("grug-far").open() end'';
        options.silent = true;
        options.desc = "Replace in all files";
      }
      {
        mode = "v";
        key = "<C-S-h>";
        action.__raw = ''function() require("grug-far").with_visual_selection() end'';
        options.silent = true;
        options.desc = "Replace selection in all files";
      }
      {
        mode = "n";
        key = "<leader>t";
        action = "<cmd>Trouble diagnostics toggle<CR>";
        options.silent = true;
        options.desc = "Toggle Trouble Diagnostics";
      }
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

    # внутри `netbox shell` shell=nbsh (команды на хосте): фильтры через пайпы, а не temp-файлы
    extraConfigLua = ''
      if vim.env.NB_HOST then vim.o.shelltemp = false end
    '';
  };
}

