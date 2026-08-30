# Issue tracker: GitHub

Issues and specs for this repository live in GitHub Issues at `ymj4023/bytebufferpool`. Use the `gh` CLI for all operations.

The local repository currently has no Git remote. Until one is configured, pass `--repo ymj4023/bytebufferpool` explicitly. Once a GitHub remote exists, `gh` may infer the repository from the working directory.

## Conventions

- Create: `gh issue create --repo ymj4023/bytebufferpool`
- Read: `gh issue view <number> --repo ymj4023/bytebufferpool --comments`
- List: `gh issue list --repo ymj4023/bytebufferpool --state open`
- Comment: `gh issue comment <number> --repo ymj4023/bytebufferpool`
- Edit labels: `gh issue edit <number> --repo ymj4023/bytebufferpool`
- Close: `gh issue close <number> --repo ymj4023/bytebufferpool`

Use `--body-file` for substantial issue bodies so Markdown remains readable and reproducible.

## Pull requests as a triage surface

**PRs as a request surface: no.**

External pull requests do not enter the triage queue unless this flag is changed to `yes`.

GitHub shares one number space across issues and pull requests. Resolve an ambiguous `#<number>` with `gh pr view`, then fall back to `gh issue view`.

## Skill operations

When a skill says "publish to the issue tracker", create a GitHub issue.

When a skill says "fetch the relevant ticket", run:

`gh issue view <number> --repo ymj4023/bytebufferpool --comments`

## Wayfinding operations

A wayfinding map is one issue labelled `wayfinder:map`. Its child tickets use `wayfinder:<type>`, where type is `research`, `prototype`, `grilling`, or `task`.

Use GitHub sub-issues and native issue dependencies when available. When they are unavailable:

- Record children in a task list in the map.
- Put `Part of #<map>` in each child.
- Put `Blocked by: #<number>` at the top of blocked tickets.
- A ticket is ready only when every blocker is closed and it has no assignee.
- Claim work with `gh issue edit <number> --add-assignee @me`.
- Resolve it by posting the answer, closing the issue, and linking the resulting decision from the map.
