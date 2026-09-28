# v0.1.1 actual artifact audit / v0.1.2 hardening

Starting main: `74a69aeba59da55ae9eef3a46a09a8353e5e8a3b`. Historical tag commit: `988720ad85af1f0d97bfe98ec4da4fcbb070beea`. Downloaded the actual public v0.1.1 assets; read RELEASE-MANIFEST.json, RELEASE-VALIDATION.md, SHA256SUMS and both VERSION.*.json, plus the tag version of tools/package/main.go.

## d2core-v0.1.1-windows-amd64.zip

SHA256: `a930ee5ae4a5f7aa21f51d7bc6ad3b0e876f4c967f950455bd46cf86d3d7ae82`

```text
BUILD.json
README.md
RELEASE_NOTES.md
d2core.exe
docs/a2s.md
docs/delivery.md
docs/local-api.md
docs/operations.md
examples/README.md
examples/launcher/README.md
examples/launcher/main.go
examples/template.linux.json
examples/template.windows.json
launcher-example.exe
```

BUILD.json:
```json
{
  "arch": "amd64",
  "buildTime": "2026-09-21T08:25:00Z",
  "gitCommit": "988720ad85af1f0d97bfe98ec4da4fcbb070beea",
  "goVersion": "1.27.1",
  "os": "windows",
  "sourceTime": "2026-09-21T08:12:51Z",
  "status": "built; artifact validation recorded separately",
  "version": "0.1.1"
}
```

## d2core-v0.1.1-linux-amd64.zip

SHA256: `58bc1e1425466dd207e90c6ab93cb9e3ee1debfc0de7992d7fca6261df1f082c`

```text
BUILD.json
README.md
RELEASE_NOTES.md
d2core
docs/a2s.md
docs/delivery.md
docs/local-api.md
docs/operations.md
examples/README.md
examples/launcher/README.md
examples/launcher/main.go
examples/template.linux.json
examples/template.windows.json
launcher-example
```

BUILD.json:
```json
{
  "arch": "amd64",
  "buildTime": "2026-09-21T08:25:00Z",
  "gitCommit": "988720ad85af1f0d97bfe98ec4da4fcbb070beea",
  "goVersion": "1.27.1",
  "os": "linux",
  "sourceTime": "2026-09-21T08:12:51Z",
  "status": "built; artifact validation recorded separately",
  "version": "0.1.1"
}
```

## Actual distribution versus candidate contract

| Material | Actual v0.1.1 ZIPs | v0.1.2 candidate |
| --- | --- | --- |
| d2core + launcher binaries | Present, platform-specific | Retained, freshly built |
| BUILD.json | Present; version/commit/time/OS/arch | Add explicit gitDirty and compatibility identity checks |
| README / RELEASE_NOTES | Present, historical 1.1 text | Candidate status and operating entry points |
| LICENSE / LICENSING.md | Absent; tag also lacks both | Mandatory in both ZIPs (main already added allowlist) |
| CHANGELOG.md | Absent | Included to distinguish historical releases and candidate |
| delivery / operations / local-api / a2s | All present | Readiness contract and candidate delivery clarified |
| examples README + both templates | Present; three map placeholder success rules, no Steam success marker | Preserve placeholders and add exact required Steam marker |
| launcher README + main.go | Present | Retained |
| third-party redistribution notices | Absent | Pinned actual dependency and Go license texts included |
| hashes / manifest / VERSION records | Separate Release assets, not ZIP entries | Per-ZIP SHA256 plus independent artifact verification report |

The tag allowlist explains the historical omissions; newer main files do not retroactively exist in old ZIPs. No historical assets are rebuilt or changed. Historical RELEASE-VALIDATION records the earlier accepted scope; this audit does not rewrite that record.

## Dependency audit

With Go 1.27.1 and CGO_ENABLED=0, `go list -deps` on both cmd/d2core and examples/launcher yields go-winio v0.6.2 on Windows, x/sys v0.10.0 on Windows/Linux, and Go's vendored x/net DNS implementation plus the Go runtime/standard library. Module cache LICENSE/PATENTS and pinned toolchain LICENSE/PATENTS/vendor license texts were inspected. MIT notice retention and BSD binary redistribution clauses motivate THIRD_PARTY_NOTICES.md. No additional third-party module occurs in these compiled dependency graphs. This is engineering distribution material review, not legal advice.

## Scope

No runtime source, protocol, schema, disk format, identity/recovery or security behavior is changed. No Dota or production node is started. Candidate build and CI results are recorded separately against the final exact commit; no v0.1.2 tag or Release is authorized.
