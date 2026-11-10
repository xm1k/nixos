{ pkgs, ... }:

let
  netbox = pkgs.buildGoModule {
    pname = "netbox";
    version = "0.2.0";
    src = ./src;
    vendorHash = "sha256-C0KX5/wUzIrS3m6deShWXTMthe9SOkkantRAt1Qd9VY=";
    env.CGO_ENABLED = "0";
    ldflags = [ "-s" "-w" ];
    nativeBuildInputs = [ pkgs.makeWrapper pkgs.installShellFiles ];
    postInstall = ''
      installShellCompletion --cmd netbox --bash <($out/bin/netbox completion)
      wrapProgram $out/bin/netbox --suffix PATH : ${pkgs.lib.makeBinPath [ pkgs.openssh pkgs.fzf ]}
    '';
    meta.mainProgram = "netbox";
  };
in
{
  environment.systemPackages = [ netbox ];
}
