{ lib, ... }:

# Shared skills and instructions for Claude Code and Codex.
# Link individual skills so each tool can keep its own installed skills.
let
  skillsDir = ../../skills;
  skills = lib.attrNames
    (lib.filterAttrs (_: t: t == "directory") (builtins.readDir skillsDir));
in
{
  home.file = lib.listToAttrs (lib.concatMap (dir: map (name: {
    name = "${dir}/${name}";
    value = {
      source = skillsDir + "/${name}";
      recursive = true;
    };
  }) skills) [ ".claude/skills" ".agents/skills" ]) // {
    ".claude/CLAUDE.md".source = ../../claude/CLAUDE.md;
    ".codex/AGENTS.md".source = ../../claude/CLAUDE.md;
    # Codex's equivalent of disable-model-invocation in SKILL.md.
    ".agents/skills/my-style/agents/openai.yaml".text = ''
      policy:
        allow_implicit_invocation: false
    '';
  };
}
