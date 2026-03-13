let
  ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIA80EZzkNzIOn6o1Pr1p6uzwifnP9uEjZu0o1/zJKDRU xm1k@nixos;
in
{
  "openai.age".publicKeys = [ xm1k ];
  "vpn.token.age".publicKeys = [ xm1k ];
}