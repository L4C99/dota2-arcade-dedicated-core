# v0.1.2 Release Hardening

Status: **final release preparation; not yet published**, artifact version `0.1.2`. Current stable remains **v0.1.1**, commit `988720ad85af1f0d97bfe98ec4da4fcbb070beea`. No v0.1.2 tag or GitHub Release is created at this gate.

## Artifact provenance and verification

- Deterministic LF checkout and fail-closed tracked-input / Git-blob provenance checks.
- Static ZIP entries checked byte-for-byte against the fixed source commit.
- Machine-generated and checked manifest/checksums; optional externally supplied SHA256 anchors.
- A single VERSION input for builder, verifier and both CI scripts; contract drift regression tests.
- Version-neutral operations paths; no Runtime changes.

## Runtime

Lifecycle, local protocol, process identity/recovery, persistence, port allocation and readiness matching engine are unchanged. Compatibility remains protocolVersion=1, template schemaVersion=1 and disk formatVersion=2. This patch adds no Runtime feature or client API.

## Release hardening

- Self-contained Windows/Linux amd64 archives include LICENSE, LICENSING.md, CHANGELOG.md and third-party redistribution texts in THIRD_PARTY_NOTICES.md, alongside operating/API documentation and examples.
- Public Dota templates require the exact `SV:  Connection to Steam servers successful.` in successAll **in addition to** verified map/script readiness. This is a template contract, not a new engine special case.
- Packaging regression checks enforce required files, archive paths, build identity, template marker and SHA256; CI checks the native packaged version on both platforms.

Steam connection success is necessary, not sufficient. Ready does not prove NAT/firewall reachability, JoinInfo mapping, Steam URI behavior or human entry. Deployment owners still perform those checks. Do not delete a slow readiness marker to obtain Ready sooner.

## Installation and limitations

See [delivery](docs/delivery.md), [operations](docs/operations.md), [templates](examples/README.md), [local API](docs/local-api.md) and [A2S](docs/a2s.md). Game binaries, SDK, runtime libraries and compatible maps must be supplied separately. Same-user local management only; ASCII paths; IPv6 wildcard dual-stack coverage remains unverified; no capacity guarantee or automatic format-1 migration. Failed instances require explicit stop/reclaim. m0-inspect remains a non-production historical diagnostic.

Artifact checks do not constitute new real-Dota acceptance. This release preparation does not start Dota or deploy production nodes. The RC2 independent delta review passed; the final version/document delta, artifacts and exact-SHA CI remain subject to the Owner Final Release Gate before any tag or GitHub Release.

## Licensing and historical assets

See [LICENSE](LICENSE), [historical scope](LICENSING.md) and [third-party notices](THIRD_PARTY_NOTICES.md). **Historical v0.1.1 tag and release assets remain unchanged.** They are not rebuilt, replaced or relabeled; the new materials are included in future builds only.
