{ pkgs, ... }:

let
  my-python = pkgs.python3.withPackages (ps: with ps; [
    pyyaml
    pydantic
    markdown
    requests
    urllib3
    pip
    setuptools
  ]);
in
{
  home.packages = [
    my-python
  ];
}
