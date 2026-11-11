{ pkgs, config, ... }:

# outlook: minimal Exchange (EWS) CLI — search / show / draft / send.
# Used by the Claude `report` skill and by any script that needs mail.
# Credentials: WORK_USER/WORK_PASS from env, else da.smirnov + agenix workpass.
let
  python = pkgs.python3.withPackages (ps: [ ps.requests ps.requests-ntlm ]);
  outlook = pkgs.stdenvNoCC.mkDerivation {
    pname = "outlook";
    version = "0.1.0";
    src = ./src;
    nativeBuildInputs = [ pkgs.makeWrapper ];
    installPhase = ''
      install -Dm755 outlook.py $out/libexec/outlook.py
      makeWrapper ${python}/bin/python3 $out/bin/outlook \
        --add-flags "-I $out/libexec/outlook.py" \
        --set-default OUTLOOK_USER da.smirnov \
        --set-default OUTLOOK_PASS_FILE ${config.age.secrets.workpass.path}
    '';
    meta.mainProgram = "outlook";
  };
in
{
  environment.systemPackages = [ outlook ];
}
