{ pkgs, inputs, ... }:

# codex with the berry TUI: Codex CLI drawn like Claude Code — `●` bullets, `⎿` results,
# `Bash(…)`/`Read(…)`/`Update(…)` tool rows, the `✻` spinner, the input in a rounded box
# and codex.b.net's blue mascot in the header. Only the TUI is changed: the agent itself
# is upstream codex as is.
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
    patches = (old.patches or [ ]) ++ [ ./berry-tui.patch ];
    # 12 rustc at once ran out of the laptop's 15 GB (codex-tui alone takes ~4 GB):
    # fewer jobs and no debug info — the build is a bit slower but survives.
    preBuild = (old.preBuild or "") + ''
      export NIX_BUILD_CORES=6
      export CARGO_PROFILE_RELEASE_DEBUG=none
    '';
  });
in
{
  environment.systemPackages = [
    (pkgs.writeShellScriptBin "codex" ''
      exec ${codex}/bin/codex --ask-for-approval never --sandbox danger-full-access \
        -c 'tui.notifications=false' \
        -c 'notify=["claude-notify","codex"]' \
        -c 'project_doc_fallback_filenames=["CLAUDE.md"]' "$@"
    '')
  ];
}
