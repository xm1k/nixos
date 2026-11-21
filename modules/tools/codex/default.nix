{ pkgs, inputs, ... }:

# codex with the berry TUI: Codex CLI drawn like Claude Code — `●` bullets, `⎿` results,
# `Bash(…)`/`Read(…)`/`Update(…)` tool rows, the `✻` spinner, the input in a rounded box
# and codex.b.net's blue mascot in the header. Desktop completion notifications also
# come from the TUI, after it checks queued input and continuing goals.
#
# Source: nixpkgs' codex package (nixpkgs-codex input, pinned by rev in flake.nix so that
# `nix flake update` does not move codex from under the patch) + ./berry-tui.patch.
# The patch comes from the fork ~/codex-berry (branch `berry` on top of tag rust-v<version>):
#   git -C ~/codex-berry diff rust-v0.167.0 --relative=codex-rs/ -- codex-rs > berry-tui.patch
# Updating codex: rebase the fork onto the new tag, regenerate the patch, set the
# nixpkgs-codex rev to a nixpkgs whose codex is that version.
let
  pkgs-codex = import inputs.nixpkgs-codex { inherit (pkgs.stdenv.hostPlatform) system; };
  codex = pkgs-codex.codex.overrideAttrs (old: {
    patches = (old.patches or [ ]) ++ [
      ./berry-tui.patch
      ./completion-notify.patch
      ./ai-bnet-image-gen.patch
    ];
    # 12 rustc at once ran out of the laptop's 15 GB (codex-tui alone takes ~4 GB),
    # and -j 6 still hit OOM (5 rustc at ~12 GB together, the big crates 2-4 GB each):
    # 3 jobs and no debug info — the build is slower but survives with a desktop open.
    preBuild = (old.preBuild or "") + ''
      export NIX_BUILD_CORES=3
      export CARGO_PROFILE_RELEASE_DEBUG=none
    '';
  });
in
{
  environment.systemPackages = [
    (pkgs.writeShellScriptBin "codex" ''
      # completion-notify.patch uses the TUI's idle decision; disable the earlier
      # legacy core hook below so queued follow-ups do not produce false "done".
      export CODEX_TUI_NOTIFY=claude-notify
      exec ${codex}/bin/codex --ask-for-approval never --sandbox danger-full-access \
        -c 'tui.notifications=false' \
        -c 'notify=[]' \
        -c 'project_doc_fallback_filenames=["CLAUDE.md"]' "$@"
    '')
  ];
}
