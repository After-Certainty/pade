# Module redirect stub (`github.com/ksteffe/pade`)

PADE moved its Go module identity to `github.com/After-Certainty/pade` starting at **v0.2.0**.

This directory holds the [Go module redirect](https://go.dev/ref/mod#module-redirect) stub for importers still requesting `github.com/ksteffe/pade`.

## Publishing (maintainers only)

After **`v0.2.0`** is tagged on `main` with `module github.com/After-Certainty/pade`:

1. Create an orphan branch or temporary worktree containing **only** [`go.mod`](go.mod) from this directory (update the `require` version if the first new-path release is not exactly `v0.2.0`).
2. Tag that commit as **`v0.1.2`** (or the next patch on the old module timeline) on the same GitHub repository.
3. Push the tag so `go get github.com/ksteffe/pade@v0.1.2` resolves to `github.com/After-Certainty/pade@v0.2.0`.

Do not merge the redirect-only tree into `main`. It exists only as a redirect tag.
