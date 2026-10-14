{ config, pkgs, ... }:

let
  containerfileDir = ./ngate;
  ageFile = "${containerfileDir}/cprocsp.tar.gz.age";

  ngate-build = pkgs.writeShellScriptBin "ngate-build" ''
    set -e

    TMPDEBS=$(mktemp -d)
    trap "rm -rf $TMPDEBS" EXIT

    echo "ngate-build: decrypting CSP packages..."
    ${pkgs.age}/bin/age -d -i ~/.ssh/id_ed25519 \
      -o "$TMPDEBS/cprocsp.tar.gz" \
      ${ageFile}
    tar -xzf "$TMPDEBS/cprocsp.tar.gz" -C "$TMPDEBS"
    rm "$TMPDEBS/cprocsp.tar.gz"

    echo "ngate-build: building image..."
    sudo podman build \
      --build-context cprocsp="$TMPDEBS" \
      -t ngate-root-image \
      ${containerfileDir}

    sudo podman rm -f ngate-root 2>/dev/null || true

    echo "ngate-build: creating container..."
    sudo podman run -d \
      --name ngate-root \
      --network host \
      --device /dev/net/tun \
      --cap-add=NET_ADMIN \
      --cap-add=NET_RAW \
      --cap-add=SYS_ADMIN \
      ngate-root-image \
      /bin/sleep infinity

    echo "ngate-build: done"
  '';
in
{
  environment.systemPackages = [ ngate-build ];
}
