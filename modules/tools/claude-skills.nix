{ lib, ... }:

# Shared skills and instructions for Claude Code and Codex.
# Skills are copied as real files, not symlinked: Kilo Code resolves symlinks
# and refuses to load SKILL.md from /nix/store ("outside project config scope").
# Each skill is replaced individually so each tool can keep its own installed skills.
let
  skillsDir = ../../skills;
  skills = lib.attrNames
    (lib.filterAttrs (_: t: t == "directory") (builtins.readDir skillsDir));
  targets = [ ".claude/skills" ".agents/skills" ];
  copySkill = dir: name: ''
    run rm -rf "$HOME/${dir}/${name}"
    run mkdir -p "$HOME/${dir}"
    run cp -rL ${skillsDir + "/${name}"} "$HOME/${dir}/${name}"
    run chmod -R u+w "$HOME/${dir}/${name}"
  '';
in
{
  home.file = {
    ".claude/CLAUDE.md".source = ../../claude/CLAUDE.md;
    ".codex/AGENTS.md".source = ../../claude/CLAUDE.md;
  };

  home.activation.copySkills = lib.hm.dag.entryAfter [ "linkGeneration" ] ''
    ${lib.concatMapStrings (dir: lib.concatMapStrings (copySkill dir) skills) targets}
    # Codex's equivalent of disable-model-invocation in SKILL.md.
    run mkdir -p "$HOME/.agents/skills/my-style/agents"
    run tee "$HOME/.agents/skills/my-style/agents/openai.yaml" >/dev/null <<'EOF'
    policy:
      allow_implicit_invocation: false
    EOF
  '';
}
