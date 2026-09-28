# v0.1.2 Release Hardening — candidate

Status: **unreleased**, candidate version `0.1.2-rc.1`. Current stable remains **v0.1.1**, commit `988720ad85af1f0d97bfe98ec4da4fcbb070beea`. No v0.1.2 tag or GitHub Release is created at this gate.

## Runtime

Lifecycle, local protocol, process identity/recovery, persistence, port allocation and readiness matching engine are unchanged. Compatibility remains protocolVersion=1, template schemaVersion=1 and disk formatVersion=2. This patch adds no Runtime feature or client API.

## Release hardening

- Self-contained Windows/Linux amd64 archives include LICENSE, LICENSING.md, CHANGELOG.md and third-party redistribution texts in THIRD_PARTY_NOTICES.md, alongside operating/API documentation and examples.
- Public Dota templates require the exact `SV:  Connection to Steam servers successful.` in successAll **in addition to** verified map/script readiness. This is a template contract, not a new engine special case.
- Packaging regression checks enforce required files, archive paths, build identity, template marker and SHA256; CI checks the native packaged version on both platforms.

Steam connection success is necessary, not sufficient. Ready does not prove NAT/firewall reachability, JoinInfo mapping, Steam URI behavior or human entry. Deployment owners still perform those checks. Do not delete a slow readiness marker to obtain Ready sooner.

## Installation and limitations

See [delivery](docs/delivery.md), [operations](docs/operations.md), [templates](examples/README.md), [local API](docs/local-api.md) and [A2S](docs/a2s.md). Game binaries, SDK, runtime libraries and compatible maps must be supplied separately. Same-user local management only; ASCII paths; IPv6 wildcard dual-stack coverage remains unverified; no capacity guarantee or automatic format-1 migration. Failed instances require explicit stop/reclaim. m0-inspect remains a non-production historical diagnostic.

Candidate checks do not constitute new real-Dota acceptance. This round does not start Dota or deploy production nodes. Independent review and explicit Owner authorization are required before any final tag or Release.

## Licensing and historical assets

See [LICENSE](LICENSE), [historical scope](LICENSING.md) and [third-party notices](THIRD_PARTY_NOTICES.md). **Historical v0.1.1 tag and release assets remain unchanged.** They are not rebuilt, replaced or relabeled; the new materials are included in future builds only.
