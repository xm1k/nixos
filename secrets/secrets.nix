let
  # Публичный SSH‑ключ пользователя xm1k
  xm1k = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIA80EZzkNzIOn6o1Pr1p6uzwifnP9uEjZu0o1/zJKDRU xm1k@nixos";
in
{
  # Для каждого файла .age указываем массив публичных ключей
  "password.age".publicKeys = [ xm1k ];
}