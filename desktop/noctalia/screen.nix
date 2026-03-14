{ pkgs, ... }:

{

	services.dbus.enable = true;

  hardware.graphics = {
    enable = true;
    extraPackages = with pkgs; [
      intel-media-driver
      libva
    ];
  };

  users.users.xm1k.extraGroups = [ "video" "render" ];

  environment.sessionVariables = {
    LIBVA_DRIVER_NAME = "iHD";
  };	

  security.wrappers = {
    gpu-screen-recorder = {
      owner = "root";
      group = "root";
      capabilities = "cap_sys_admin,cap_sys_nice+ep";
      source = "${pkgs.gpu-screen-recorder}/bin/gpu-screen-recorder";
    };
    gsr-kms-server = {
      owner = "root";
      group = "root";
      capabilities = "cap_sys_admin+ep";
      source = "${pkgs.gpu-screen-recorder}/bin/gsr-kms-server";
    };
  };
}
