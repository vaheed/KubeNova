## Summary
- _Describe the change in one or two sentences._

## Testing
- [ ] `go fmt ./...`
- [ ] `go vet ./...`
- [ ] `go test ./... -count=1`
- [ ] `gosec ./...`
- [ ] `trivy config --severity HIGH,CRITICAL --format table --exit-code 1 deploy`
- [ ] `make test-e2e`

## Checklist
- [ ] Docs updated (`README.md`, `docs/tests.md`, `docs/site/*` as needed)
- [ ] CHANGELOG entry added and version bumped (SemVer)
- [ ] CI configuration reflects any new dependencies or steps
- [ ] No secrets or credentials committed
