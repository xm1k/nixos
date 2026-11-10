{ pkgs, ... }:

# jira: my Jira Server/DC CLI (issues on the board, background queue, weekly
# report). Source: ./src. After changing go.mod, update vendorHash (build
# once with lib.fakeHash and take the hash from the error).
let
  jira = pkgs.buildGoModule {
    pname = "jira";
    version = "0.3.0";
    src = ./src;
    vendorHash = "sha256-7K17JaXFsjf163g5PXCb5ng2gYdotnZ2IDKk8KFjNj0=";
    env.CGO_ENABLED = "0";
    ldflags = [ "-s" "-w" ];
    nativeBuildInputs = [ pkgs.makeWrapper pkgs.installShellFiles ];
    postInstall = ''
      installShellCompletion --cmd jira \
        --bash <($out/bin/jira completion bash) \
        --zsh <($out/bin/jira completion zsh) \
        --fish <($out/bin/jira completion fish)
      # gum draws the pickers and prompts, xdg-open opens issues in the browser.
      wrapProgram $out/bin/jira --suffix PATH : ${pkgs.lib.makeBinPath [ pkgs.gum pkgs.xdg-utils ]}
    '';
    meta.mainProgram = "jira";
  };
in
{
  environment.systemPackages = [ jira ];
}
