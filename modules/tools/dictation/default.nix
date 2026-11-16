{ pkgs, lib, ... }:
let
  python = pkgs.python3.withPackages (ps: [ ps.pygobject3 ps.pycairo ]);
  dictation = pkgs.stdenv.mkDerivation {
    pname = "berry-dictation";
    version = "1";
    dontUnpack = true;
    nativeBuildInputs = with pkgs; [ wrapGAppsHook3 gobject-introspection makeWrapper ];
    buildInputs = with pkgs; [ gtk3 gtk-layer-shell ];
    dontWrapGApps = true;
    installPhase = ''
      install -Dm644 ${./dictation.py} $out/share/berry-dictation/dictation.py
      mkdir -p $out/bin
    '';
    postFixup = ''
      makeWrapper ${python}/bin/python3 $out/bin/berry-dictation \
        "''${gappsWrapperArgs[@]}" \
        --prefix PATH : ${lib.makeBinPath [ pkgs.pipewire pkgs.whisper-cpp pkgs.wl-clipboard ]} \
        --add-flags $out/share/berry-dictation/dictation.py
    '';
  };
  toggle = pkgs.writeShellApplication {
    name = "dictation-toggle";
    runtimeInputs = [ pkgs.systemd pkgs.util-linux ];
    text = ''
      exec 9>"$XDG_RUNTIME_DIR/berry-dictation.lock"
      flock 9
      if systemctl --user is-active --quiet berry-dictation.service; then
        systemctl --user kill --kill-whom=main --signal=SIGUSR1 berry-dictation.service
      else
        # A newly installed unit may not be loaded yet; reset-failed then
        # returns an error, which must not prevent its first start.
        systemctl --user reset-failed berry-dictation.service || true
        systemctl --user start berry-dictation.service
      fi
    '';
  };
in {
  home.packages = [ dictation toggle ];
  systemd.user.services.berry-dictation = {
    Unit = {
      Description = "Local streaming voice dictation (Ctrl+Alt+R)";
      PartOf = [ "graphical-session.target" ];
      After = [ "graphical-session.target" ];
    };
    Service = {
      Type = "notify";
      ExecStart = "${dictation}/bin/berry-dictation";
      UMask = "0077";
      TimeoutStopSec = 5;
    };
  };
}
