{ pkgs, ... }:

{

    programs.steam = {
        enable = true;
        remotePlay.openFirewall = true;
        dedicatedServer.openFirewall = true;
        localNetworkGameTransfers.openFirewall = true;
        package = pkgs.steam.override {
        extraArgs = "-cef-disable-gpu -no-cef-sandbox";
        };
    };

}