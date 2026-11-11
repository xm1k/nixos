{ ... }:

{
  # Пишет ~/.config/git/config; ~/.gitconfig (user, safe) и /etc/gitconfig (креды) тоже читаются.
  programs.git = {
    enable = true;
    settings = {
      # `git push` в новой ветке сам делает -u origin <ветка>
      push.autoSetupRemote = true;
    };
  };

  programs.bash.initExtra = ''
    # gcp [сообщение] — add -A, commit, push. Без сообщения откроется редактор.
    gcp() {
      git add -A || return
      if [ $# -gt 0 ]; then git commit -m "$*"; else git commit; fi || return
      git push
    }

    # gcap [сообщение] — add -A, commit --amend, push --force-with-lease.
    # Без сообщения текст коммита не меняется.
    gcap() {
      git add -A || return
      if [ $# -gt 0 ]; then git commit --amend -m "$*"; else git commit --amend --no-edit; fi || return
      git push --force-with-lease
    }
  '';

  # Частые опечатки (по activity-report)
  programs.bash.shellAliases = {
    jria = "jira";
    jrai = "jira";
    cear = "clear";
    claer = "clear";
    clera = "clear";
    gti = "git";
    got = "git";
    nivm = "nvim";
    nvmi = "nvim";
    vnim = "nvim";
  };
}
