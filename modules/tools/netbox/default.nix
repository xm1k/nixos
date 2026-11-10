{ pkgs, ... }:

# netbox: поиск хостов из ansible inventory (NetBox) и ssh без ввода пароля.
# Пароль берётся из $WORK_PASS, пользователь — из $WORK_ADMIN_USER.
# Исходник: ./netbox (один python-файл).
let
  # pyyaml — для yaml-снапшота; ansible-core + pytz/requests/packaging —
  # для `netbox update --live` (плагин netbox.netbox.nb_inventory из ansible-galaxy).
  python = pkgs.python3.withPackages (ps: with ps; [
    pyyaml
    ansible-core
    pytz
    requests
    packaging
  ]);

  netbox = pkgs.stdenvNoCC.mkDerivation {
    pname = "netbox-cli";
    version = "0.1.0";
    src = ./netbox;
    dontUnpack = true;
    buildInputs = [ python ];
    nativeBuildInputs = [ pkgs.makeWrapper pkgs.installShellFiles ];
    installPhase = ''
      install -Dm755 $src $out/bin/netbox
    '';
    postFixup = ''
      # openssh — сам ssh (пароль через SSH_ASKPASS), fzf — выбор хоста,
      # ansible-inventory из python-окружения — для --live.
      wrapProgram $out/bin/netbox \
        --prefix PATH : ${pkgs.lib.makeBinPath [ pkgs.openssh pkgs.fzf python ]}
      installShellCompletion --cmd netbox --bash <($out/bin/netbox completion)
    '';
    meta.mainProgram = "netbox";
  };
in
{
  environment.systemPackages = [ netbox pkgs.fzf ];
}
