{ pkgs, ... }:

let
  packages = pkgs.python3.withPackages (ps: with ps; [
    ansible-core
    pyyaml
    pydantic
    markdown
    requests
    urllib3
    pip
    setuptools
    pyperclip
    gitpython
  ]);
in
{
  home.packages = [
    packages
  ];
}
