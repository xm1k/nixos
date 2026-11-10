{ pkgs, ... }:

# `work` — интерактивный запуск контейнеров rei_ansible / rei_ansible_molecule.
# Секреты: agenix work_env (secrets/work_env.age), VAULT_TOKEN — из ~/.vault-token.
let
  work = pkgs.writeShellApplication {
    name = "work";
    runtimeInputs = with pkgs; [ gum coreutils findutils gnugrep curl jq yq-go ];
    text = builtins.readFile ./work.sh;
  };
in
{
  home.packages = [ work ];
}
