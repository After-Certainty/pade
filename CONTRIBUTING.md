# Contributing

Thanks for interest in PADE. The project is exploratory (draft Intent / Consumer / Broker specs with a Go reference implementation; see [README.md](README.md) and [spec/README.md](spec/README.md)).

PADE is a draft interoperability contract plus a Go reference implementation—not an industry standard or certification program.

## What kind of change is this?

When proposing work, note which surface it affects:

| Surface | Examples | Update |
|---------|----------|--------|
| **Intent Specification** | `pade.yaml` fields, [spec/pade.schema.json](spec/pade.schema.json), capability declaration semantics | [spec/intent.md](spec/intent.md), examples |
| **Consumer Specification** | How Intent is read/validated/requested; broker client expectations | [spec/consumer.md](spec/consumer.md) |
| **Broker Specification** | Wire protocol, authn/authz/materialization boundaries | [spec/broker.md](spec/broker.md) |
| **Reference implementation only** | Go packages, provider adapters, dogfood scripts, CLI UX that does not change the contract | [DESIGN.md](DESIGN.md) / [docs/go-reference.md](docs/go-reference.md) as needed |

Protocol-affecting changes should be called out in the PR. Prefer documenting experimental extensions before treating them as normative. There is no heavy governance process while PADE remains exploratory.

## Before you change code or schema

1. Read [README.md](README.md), [spec/README.md](spec/README.md), [RFC.md](RFC.md), and [DESIGN.md](DESIGN.md) (including DevPod-first revisions and the Intent/Consumer/Broker model).
2. Prefer composing Dev Containers / DevPod / existing secret managers over new PADE subsystems.
3. Keep secrets out of manifests, logs, plan output, and git.
4. Do not put provider-specific IDs, broker endpoints, or tokens into portable Intent.

## Development

Requires **Go 1.22+** (primary toolchain **Go 1.26.6** via [mise](https://mise.jdx.dev/)). Older toolchains (for example Homebrew Go 1.13) fail on `embed` and modern module requirements.

```bash
# Install mise (https://mise.jdx.dev/), then:
mise install          # Go 1.26.6 from mise.toml / mise.lock
mise tasks            # discover tasks
mise run test
mise run ci           # local mirror of GitHub unit + smoke jobs (not container/CodeQL/DevPod)

# Or invoke Go directly (after mise install / mise activate):
go test ./...
go run ./cmd/pade validate -f spec/examples/web-app.yaml
go run ./cmd/pade plan -f spec/examples/web-app.yaml
```

Pull requests and pushes to `main` run [`.github/workflows/ci.yml`](.github/workflows/ci.yml). GitHub additionally runs a Go 1.22 compatibility job, container smoke, CodeQL, and pull-request dependency review.

## Pull requests

- Branch from current `main`. Open PRs that target `main` unless the maintainer asks otherwise.
- Do not merge PRs or push to `main` unless the maintainer explicitly asks to merge *and* CI on that PR is green.
- Keep changes focused and explained against the design hypotheses.
- Update docs and [spec/](spec/) when behavior or schema changes.
- Do not commit `.env`, credential files, or `.pade/` state.

## License

By contributing, you agree that your contributions are licensed under the Apache License 2.0.
