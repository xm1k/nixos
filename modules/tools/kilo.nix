{ config, lib, pkgs, ... }:

# Kilo Code CLI (`kilo`).
# - The binary comes from upstream releases: pkgs.kilo in nixpkgs is marked
#   broken (nixpkgs#542516). It runs unpatched thanks to programs.nix-ld.
# - The whole config (provider keys, MCP tokens) is an agenix secret, passed
#   via KILO_CONFIG. ~/.config/kilo/kilo.jsonc stays writable for the
#   settings Kilo saves itself; keep secrets out of it.
# - Auto-approve lives in that secret too: every "ask" is "allow", explicit
#   "deny" rules stay. KILO_CONFIG overrides ~/.config/kilo/kilo.jsonc key by
#   key, so asks there no longer prompt. (Not KILO_CONFIG_CONTENT: JSON in an
#   env var gets mangled by /etc/pam/environment and set-environment quoting.)
#   Edit: cd /etc/nixos/secrets && agenix -e kilo_config.age

let
  kilo = pkgs.stdenvNoCC.mkDerivation rec {
    pname = "kilo";
    version = "7.8.27";
    src = pkgs.fetchurl {
      url = "https://github.com/Kilo-Org/kilocode/releases/download/v${version}/kilo-linux-x64.tar.gz";
      hash = "sha256-TAiqRt2HRToyn0iw8A000Pp7jwrJEP6JN4FNToWC3gg=";
    };
    unpackPhase = ''
      mkdir source
      tar xzf $src -C source
      cd source
    '';
    # A bun-compiled binary: its payload must not be stripped or patched.
    dontStrip = true;
    dontPatchELF = true;
    dontFixup = true;
    installPhase = ''
      mkdir -p $out/lib/kilo $out/bin
      cp -r . $out/lib/kilo/
      ln -s $out/lib/kilo/kilo $out/bin/kilo
    '';
    meta = {
      description = "Kilo Code CLI";
      homepage = "https://kilo.ai";
      mainProgram = "kilo";
      platforms = [ "x86_64-linux" ];
    };
  };

in
{
  age.secrets.kilo_config = {
    file = ../../secrets/kilo_config.age;
    owner = "xm1k";
    group = "users";
    mode = "0400";
  };

  environment.systemPackages = [ kilo ];

  # sessionVariables reach graphical sessions too (VS Code / Kilo extension).
  environment.sessionVariables = {
    KILO_CONFIG = config.age.secrets.kilo_config.path;
  };
}
