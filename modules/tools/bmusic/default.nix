{ pkgs, ... }:

# bmusic: music.b.net in the terminal — TUI plus a CLI that drives a background
# player daemon (mpv over IPC, autostarted). Source: ./src. After changing go.mod,
# update vendorHash (build once with lib.fakeHash and take the hash from the error).
let
  bmusic = pkgs.buildGoModule {
    pname = "bmusic";
    version = "0.1.0";
    src = ./src;
    vendorHash = "sha256-5QCeqChpoxtsT/pQv7jvOBBaoADlNHQ0OC7E2wJyEwM=";
    env.CGO_ENABLED = "0";
    ldflags = [ "-s" "-w" ];
    nativeBuildInputs = [ pkgs.makeWrapper pkgs.installShellFiles ];
    postInstall = ''
      installShellCompletion --cmd bmusic --bash <($out/bin/bmusic completion bash)
      wrapProgram $out/bin/bmusic --set-default BMUSIC_MPV ${pkgs.mpv}/bin/mpv
    '';
    meta.mainProgram = "bmusic";
  };
in
{
  environment.systemPackages = [ bmusic ];
}
