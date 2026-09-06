# Development and releases

Use the Go version specified in [`go.mod`](../go.mod).

```sh
go test ./...
go vet ./...
go build ./cmd/oku
```

## Branch and release flow

Feature branches are opened as pull requests into `develop` (CI runs `go test ./...` on every PR). `Master` is the release branch and keeps the released history.

To release:

```bash
git checkout Master
git pull origin Master
git merge origin/develop
go test ./...
goreleaser release --snapshot --clean
git tag vX.Y.Z
git push origin Master
git push origin vX.Y.Z
```

Pushing a `v*` tag runs the GoReleaser workflow, publishes the GitHub release, and updates the Homebrew cask.


