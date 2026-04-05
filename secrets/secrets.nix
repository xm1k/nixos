let
  xm1k = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIA80EZzkNzIOn6o1Pr1p6uzwifnP9uEjZu0o1/zJKDRU xm1k@nixos";
  users = [ xm1k ];
in
{
  "password.age".publicKeys = [ xm1k ];
	"freeportpass.age".publicKeys = [ xm1k ];
	"workpass.age".publicKeys = [ xm1k ];
	"github_token.age".publicKeys = [ xm1k ];
	"gitlab_token.age".publicKeys = [ xm1k ];
}
