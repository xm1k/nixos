{ config, ... }:

{
    age.secrets = {
        password = {
            file = ./password.age;
            owner = "xm1k";
            group = "xm1k";
            mode = "0400";
        };
    };
}