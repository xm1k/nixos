{ config, pkgs, ... }:

let
  k0s = pkgs.writeShellScriptBin "k0s" ''
    CONFIGS=$(ls ~/.kube-conf/)

    FILE=$(echo "$CONFIGS" | gum choose --height 10)

    if [ -n "$FILE" ]; then
        k9s --kubeconfig "/home/xm1k/.kube-conf/$FILE"
    fi  '';
  kfg = pkgs.writeShellScriptBin "kfg" ''
    CONFIGS=$(ls ~/.kube-conf/)

    FILE=$(echo "$CONFIGS" | gum choose --height 10)

    if [ -n "$FILE" ]; then
        cp "/home/xm1k/.kube-conf/$FILE" "/home/xm1k/.kube/config"
    fi  '';
in
{
  home.packages = [
    k0s
    kfg
  ];
}
