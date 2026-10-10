{ pkgs, lib, ... }:

let
  version = "02.08.02.61";
  date    = "20260820225108";

  src = pkgs.fetchurl {
    url  = "https://github.com/bambulab/BambuStudio/releases/download/v${version}/BambuStudio_ubuntu22.04-v${version}-${date}.AppImage";
    hash = "sha256-aUJqWWgldFkVkPUckT04ibqhklLRyOn8riSX6w7Wv5I=";
  };

  app = pkgs.appimageTools.extractType2 {
    pname = "bambu-studio";
    inherit version src;
  };

  aptPackages = lib.concatStringsSep " " [
    "libgtk-3-0"
    "libwebkit2gtk-4.1-0"
    "libglu1-mesa"
    "libgstreamer1.0-0"
    "libgstreamer-plugins-base1.0-0"
    "libavcodec-extra"
    "libfontconfig1"
    "libcairo2"
    "libpango-1.0-0"
    "libpangocairo-1.0-0"
    "libgdk-pixbuf-2.0-0"
    "libgl1"
    "libssl3"
    "libcurl4"
  ];

in {
  environment.systemPackages = [
    pkgs.distrobox

    (pkgs.writeShellScriptBin "bambu-setup" ''
      set -e
      echo "→ Creating distrobox container 'bambu' (ubuntu:22.04)..."
      distrobox create \
        --name bambu \
        --image ubuntu:22.04 \
        --volume /nix:/nix \
        --yes 2>/dev/null || true

      echo "→ Installing dependencies..."
      distrobox enter bambu -- sudo bash -c \
        "apt-get update -qq && apt-get install -y --no-install-recommends ${aptPackages}"

      echo "✓ Bambu Studio ready. Run 'bambu-studio' to launch."
    '')

    (pkgs.writeShellScriptBin "bambu-studio" ''
      if ! distrobox list 2>/dev/null | grep -q "bambu"; then
        echo "First run: setting up container..."
        bambu-setup
      fi

      distrobox enter bambu -- ${app}/AppRun "$@" \
        > /tmp/bambu-studio.log 2>&1 &

      disown
    '')
  ];
}
