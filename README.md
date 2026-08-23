# Go module redirect (`github.com/ksteffe/pade`)

This branch exists **only** to publish tag **`v0.1.2`** as a [Go module redirect](https://go.dev/ref/mod#module-redirect) stub.

PADE’s canonical module path from **v0.2.0** onward is **`github.com/After-Certainty/pade`**. This tree is not a buildable copy of PADE — it contains a single `go.mod` so importers on the old path can forward to the new module.

## Tagging (maintainers)

**Prerequisite:** `v0.2.0` must already be tagged on `main` with `module github.com/After-Certainty/pade`.

```bash
git checkout release/ksteffe-module-redirect-v0.1.2
# Confirm go.mod require version matches the released new-path tag (default: v0.2.0)
git tag -a v0.1.2 -m "Redirect github.com/ksteffe/pade to github.com/After-Certainty/pade v0.2.0"
git push origin v0.1.2
```

Verify:

```bash
GOPROXY=direct go list -m -json github.com/ksteffe/pade@v0.1.2
```

**Do not merge this branch into `main`.** Development continues on `main` with the After-Certainty module path.

See [docs/release.md](https://github.com/After-Certainty/pade/blob/main/docs/release.md) on `main` for the full migration checklist.
