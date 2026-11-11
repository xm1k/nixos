{ pkgs, ... }:

let
  netbox = pkgs.buildGoModule {
    pname = "netbox";
    version = "0.3.0";
    src = ./src;
    vendorHash = "sha256-C0KX5/wUzIrS3m6deShWXTMthe9SOkkantRAt1Qd9VY=";
    env.CGO_ENABLED = "0";
    ldflags = [ "-s" "-w" ];
    nativeBuildInputs = [ pkgs.makeWrapper pkgs.installShellFiles ];
    postInstall = ''
      installShellCompletion --cmd netbox --bash <($out/bin/netbox completion)
      wrapProgram $out/bin/netbox --suffix PATH : ${pkgs.lib.makeBinPath [ pkgs.openssh pkgs.fzf pkgs.sshfs ]}
      # nbsh — шим-шелл для `netbox shell` (определяется по argv[0])
      ln -s netbox $out/bin/nbsh
    '';
    meta.mainProgram = "netbox";
  };
in
{
  environment.systemPackages = [ netbox ];
}
