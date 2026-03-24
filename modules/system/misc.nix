# Miscellaneous system configuration module
{ config, lib, pkgs, ... }:

{
  # RAM managing
  boot.tmp.useTmpfs = true;
  boot.tmp.tmpfsSize = "50%"; # выделяем до половины ОЗУ под /tmp
  services.psd.enable = true;

  # Nix settings
  nix.settings = {
    show-trace = true;
    extra-experimental-features = [ "nix-command" "flakes" ];
    extra-sandbox-paths = [];
    max-jobs = 4;
  };

  # Timezone
  time.timeZone = "Europe/Moscow";

  # Environment variables
  environment.variables = {
    SSL_CERT_FILE = "${pkgs.cacert}/etc/ssl/certs/ca-bundle.crt";
    SSL_CERT_DIR = "${pkgs.cacert}/etc/ssl/certs";
    NIX_SSL_CERT_FILE = "${pkgs.cacert}/etc/ssl/certs/ca-bundle.crt";
    NIXOS_OZONE_WL = "1";
    DISPLAY = ":0";
  };

  # Fonts
  fonts.packages = with pkgs; [
    nerd-fonts.jetbrains-mono
    nerd-fonts.fira-code
  ];

  # Security
  security.pki.certificateFiles = [ "${pkgs.cacert}/etc/ssl/certs/ca-bundle.crt" ];
}