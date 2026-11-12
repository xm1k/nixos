{ lib, ... }:

# Claude Code skills from /etc/nixos/skills → ~/.claude/skills/<name>.
# Only the skill subdirectories are linked: ~/.claude/skills/synced
# (skills synced from claude.ai) stays writable.
let
  skillsDir = ../../skills;
  skills = lib.attrNames
    (lib.filterAttrs (_: t: t == "directory") (builtins.readDir skillsDir));
in
{
  home.file = lib.listToAttrs (map (name: {
    name = ".claude/skills/${name}";
    value = {
      source = skillsDir + "/${name}";
      recursive = true;
    };
  }) skills) // {
    ".claude/CLAUDE.md".source = ../../claude/CLAUDE.md;
  };
}
