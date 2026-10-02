<p align="center">
  <a href="./CONTRIBUTING.md">English</a> /
  <a href="./CONTRIBUTING-ru.md">Русский</a> /
  <a href="./CONTRIBUTING-zh-cn.md">简体中文</a>
</p>

# Contributing to Gamaj Bot

Thanks for helping. This project has a small set of non-negotiable rules; read
them before you write any code, because a pull request that breaks them will not
be merged.

## Red lines

1. **Only the Gamaj name.** No reference to any other bot or panel project, in
   code, comments, UI text, docs, or assets. The automated env naming guard
   fails the build if such a name appears.
2. **No agent traces.** Do not mention AI assistants, agents, or automation
   tools in code, commit messages, or documentation. Commits must look like
   normal developer work.
3. **One install path: native binary + systemd.** No Docker and no install-mode
   switch. The installer in `scripts/install.sh` expects a built `gamajbot`
   binary and configures the `gamaj-bot` systemd service.
4. **Every environment variable starts with `GAMAJ_`.** Runtime configuration
   lives in the JSON config file (`panel_url`, `api_key`, `bot_token`,
   `admin_id`), not in free-form environment variables; anything read from the
   process environment must pass the naming guard.
5. **Secrets never live in the repository.** The installer creates the config
   with placeholders and `chmod 600`; keep it that way.
6. **Docs are self-contained.** Documentation must not send readers to GitHub or
   another site "to read more"; the content belongs in this repository (and in
   the `Web/` docs site).

## Workflow

1. Fork or branch from `Asli`.
2. Keep changes focused; one topic per pull request.
3. Run the checks that apply to what you touched:

```bash
cd Bot
gofmt -l .                       # no output
go build ./... && go vet ./...
go test ./internal/platform/envguard/

# installer changes:
bash -n scripts/install.sh scripts/manage.sh
```

## Commit messages

Short, imperative, and about the change, for example:

```
Reject webhook updates with an unknown secret
Rate-limit admin commands per chat
```

Do not add tool footers, generated-by lines, or co-author trailers. The commit
author and committer must be your own Git identity.

## Code style

- Go: `gofmt`, `go vet`, error wrapping with `%w`, table-driven tests next to
  the package they cover.
- Shell: `set -euo pipefail`, no external dependency beyond the usual POSIX
  tools.
- Telegram UX: user-facing strings stay consistent with the panel dashboard
  wording; keyboard layouts must survive both compact and wide screens.

## Reporting problems

Include: the bot version (release tag or `gamajbot -version`), the OS and
architecture, journal output (`journalctl -u gamaj-bot -n 100`), and the
minimal steps to reproduce. Never paste real bot tokens, API keys, or admin
chat IDs into an issue — rotate the token and share redacted logs. Security
issues go to the maintainers privately.
