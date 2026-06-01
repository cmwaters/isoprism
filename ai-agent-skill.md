# Isoprism Agent Skill Setup

The portable skill lives at:

```text
skills/isoprism/
```

It follows the shared Agent Skills folder shape:

```text
isoprism/
  SKILL.md
  agents/openai.yaml
  references/
    project-reference.md
```

Keep the folder name `isoprism` so the invocation name is predictable.

## Codex

Install for all Codex sessions by copying this skill into your Codex skills directory:

```bash
mkdir -p "${CODEX_HOME:-$HOME/.codex}/skills"
cp -R /path/to/isoprism/skills/isoprism "${CODEX_HOME:-$HOME/.codex}/skills/isoprism"
```

For local development against this repo, a symlink keeps edits live:

```bash
mkdir -p "${CODEX_HOME:-$HOME/.codex}/skills"
ln -s /path/to/isoprism/skills/isoprism "${CODEX_HOME:-$HOME/.codex}/skills/isoprism"
```

If the destination already exists, rename or remove the old destination first after confirming it is not carrying local changes.

Then start a new Codex session. Invoke explicitly with:

```text
Use $isoprism to review my current changes.
```

Codex can also invoke the skill implicitly when the task mentions Isoprism, semantic PR review, `isoprism diff`, `isoprism serve`, graph annotations, or the Isoprism source repo.

## Claude Code

Claude Code supports personal and project skills. The command name comes from the skill directory name, so this skill is invoked as `/isoprism`.

Install as a personal skill available in all projects:

```bash
mkdir -p ~/.claude/skills
cp -R /path/to/isoprism/skills/isoprism ~/.claude/skills/isoprism
claude
```

Install as a project skill available only inside one repository:

```bash
mkdir -p .claude/skills
cp -R /path/to/isoprism/skills/isoprism .claude/skills/isoprism
claude
```

For active skill development, use a symlink instead of a copy:

```bash
mkdir -p .claude/skills
ln -s /path/to/isoprism/skills/isoprism .claude/skills/isoprism
```

Claude Code detects changes to existing skill files while running. If the `.claude/skills` directory did not exist when Claude started, restart Claude Code so the directory is watched.

Invoke explicitly with:

```text
/isoprism review my current branch
```

Or ask naturally:

```text
Use Isoprism to inspect the semantic diff before I push.
```

Claude Code reads `SKILL.md` and supporting files. Codex may also read `agents/openai.yaml` for UI metadata.

Official Claude Code reference: https://code.claude.com/docs/en/skills
