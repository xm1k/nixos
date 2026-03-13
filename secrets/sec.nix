{ config, ... }:

{
    age.secrets = {
        password = {
            file = ./secrets/password.age;
            owner = "xm1k";
            group = "xm1k";
            mode = "0400";
        };
    };
}