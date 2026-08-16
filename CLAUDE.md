# CLAUDE.md

Follow `AGENTS.md` first.

## Claude-specific guidance

- Be concise and implementation-focused.
- Before editing, inspect the files that own the behavior.
- Make the smallest correct diff.
- Avoid new dependencies unless they clearly remove more code than they add.
- For SaaS features, check tenant isolation and authorization paths.
- For AI features, avoid autonomous destructive actions; require user confirmation.

## Agent skills

### Issue tracker

Issues are tracked in GitHub Issues using the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Domain docs

This is a single-context repo: read root `CONTEXT.md` and root `docs/adr/` when present. See `docs/agents/domain.md`.

## Project identity

Project name: **AIVO RMS**

Description: open-source AI-powered restaurant management system SaaS.
