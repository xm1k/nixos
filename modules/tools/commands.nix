{ config, pkgs, ... }:

{
  programs.bash.initExtra = ''
    proxy() {
      export http_proxy="http://da.smirnov:$FREE_PORT_PASS@10.1.252.242:9999"
      export HTTPS_PROXY="http://da.smirnov:$FREE_PORT_PASS@10.1.252.242:9999"
      export ALL_PROXY="http://da.smirnov:$FREE_PORT_PASS@10.1.252.242:9999"
      export all_proxy="http://da.smirnov:$FREE_PORT_PASS@10.1.252.242:9999"
      echo "proxying..."
    }
  '';
}
