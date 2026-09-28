"""Maintainer-only, offline artifact verification. Never publishes or starts Dota."""
import sys
if not __debug__:
    raise RuntimeError("artifact verification requires assertions enabled (no -O)")

import argparse
import hashlib
import json
import platform
import re
import subprocess
import tempfile
import zipfile
from pathlib import Path

# Independent distribution contract: omissions in the builder must fail verification.
REQUIRED = {
    'LICENSE', 'LICENSING.md', 'THIRD_PARTY_NOTICES.md', 'CHANGELOG.md',
    'README.md', 'RELEASE_NOTES.md', 'BUILD.json',
    'docs/delivery.md', 'docs/operations.md', 'docs/local-api.md', 'docs/a2s.md',
    'examples/README.md', 'examples/template.windows.json',
    'examples/template.linux.json', 'examples/launcher/README.md',
    'examples/launcher/main.go',
}
REPOSITORY = Path(__file__).resolve().parents[2]
MARKER = 'SV:  Connection to Steam servers successful.'


def git_blob(repo, commit, name):
    return subprocess.check_output(['git', '--no-replace-objects', '-C', str(repo), 'show', f'{commit}:{name}'])


def check_contract(repo, commit):
    # Read the builder contract from the same immutable revision, not the worktree.
    source = git_blob(repo, commit, 'tools/package/main.go').decode('utf-8')
    block = re.search(r'var releaseFiles = \[\]string\{(.*?)\n\}', source, re.S)
    assert block, 'builder releaseFiles contract not found'
    names = re.findall(r'"([^"\n]+)"', block.group(1))
    assert len(names) == len(set(names)), 'duplicate builder contract entry'
    assert set(names) | {'BUILD.json'} == REQUIRED, 'builder/verifier contracts differ'


def verify(directory, commit, version, native=False, repo=REPOSITORY, expected_sha256=None):
    assert re.fullmatch('[0-9a-f]{40}', commit), 'full commit SHA required'
    resolved = subprocess.check_output(['git', '--no-replace-objects', '-C', str(repo), 'rev-parse', commit+'^{commit}'], text=True).strip()
    assert resolved == commit
    assert git_blob(repo, commit, 'tools/package/VERSION').decode('ascii').strip() == version, 'version differs from fixed source VERSION'
    check_contract(repo, commit)
    sources = {name: git_blob(repo, commit, name) for name in REQUIRED - {'BUILD.json'}}
    if expected_sha256 is not None:
        assert set(expected_sha256) == {f'd2core-v{version}-{os}-amd64.zip' for os in ('windows','linux')}, 'supply both external digests'
    results = []
    for target in ('windows', 'linux'):
        suffix = '.exe' if target == 'windows' else ''
        path = directory / f'd2core-v{version}-{target}-amd64.zip'
        digest = hashlib.sha256(path.read_bytes()).hexdigest()
        expected = path.with_suffix('.zip.sha256').read_text().split()
        assert expected == [digest, path.name], f'checksum mismatch: {path}'
        if expected_sha256 is not None:
            assert expected_sha256[path.name] == digest, f'external digest mismatch: {path.name}'
        with zipfile.ZipFile(path) as z:
            names = z.namelist()
            assert len(names) == len(set(n.lower() for n in names)), 'duplicate archive path'
            assert set(names) == REQUIRED | {'d2core'+suffix, 'launcher-example'+suffix}, names
            assert z.testzip() is None, 'ZIP CRC failure'
            for name, source in sources.items():
                assert z.read(name) == source, f'ZIP/Git blob provenance mismatch: {name}'
            build = json.loads(z.read('BUILD.json'))
            for key, value in dict(version=version, gitCommit=commit, gitDirty=False,
                                   os=target, arch='amd64', protocolVersion=1,
                                   schemaVersion=1, formatVersion=2, goVersion='1.27.1').items():
                assert build[key] == value, (key, build[key], value)
            assert re.fullmatch('[0-9a-f]{40}', commit)
            for template in ('windows', 'linux'):
                config = json.loads(z.read(f'examples/template.{template}.json'))
                assert config['schemaVersion'] == 1
                assert MARKER in config['readiness']['successAll']
                assert len(config['readiness']['successAll']) == 4
            for name in names:
                if not name.endswith('.md'):
                    continue
                for link in re.findall(r'\]\(([^)]+)\)', z.read(name).decode('utf-8')):
                    link = link.split('#')[0]
                    if not link or '://' in link:
                        continue
                    import posixpath
                    assert posixpath.normpath(posixpath.join(posixpath.dirname(name), link)) in names, (name, link)
            smoke = 'SKIPPED (other OS)'
            with tempfile.TemporaryDirectory(prefix='d2core-package-') as temp:
                z.extractall(temp)  # exact, flat-root allowlist already checked
                if native and platform.system().lower() == target:
                    binary = Path(temp) / ('d2core'+suffix)
                    binary.chmod(0o755)
                    response = json.loads(subprocess.check_output([str(binary), 'version', '--json'], text=True))
                    assert response['ok'] is True and response['protocolVersion'] == 1, response
                    value = response['result']
                    for key in ('version', 'gitCommit', 'gitDirty', 'buildTime', 'protocolVersion', 'schemaVersion', 'formatVersion'):
                        assert value[key] == build[key], (key, value, build)
                    assert value['goVersion'] == 'go'+build['goVersion']
                    smoke = 'PASS'
            results.append(dict(file=path.name, sha256=digest, bytes=path.stat().st_size,
                                entries=sorted(names), build=build, nativeVersion=smoke))
    assert results[0]['build']['buildTime'] == results[1]['build']['buildTime'], 'platform buildTime mismatch'
    return results


def manifest_for(results):
    first = results[0]['build']
    return dict(status='unreleased candidate; independent review gate',
                version=first['version'], gitCommit=first['gitCommit'], buildTime=first['buildTime'],
                protocolVersion=1, schemaVersion=1, formatVersion=2,
                artifacts=[{k: v for k, v in item.items() if k != 'nativeVersion'} for item in results])


def metadata(directory, results, write=False):
    expected = manifest_for(results)
    manifest = directory / 'RELEASE-MANIFEST.json'
    sums = directory / 'SHA256SUMS'
    checksum_text = ''.join(item['sha256']+'  '+item['file']+'\n' for item in results)
    if write:
        # Exclusive creation: never silently overwrite a reviewed manifest.
        with manifest.open('x', encoding='utf-8', newline='\n') as f:
            json.dump(expected, f, indent=2)
            f.write('\n')
        with sums.open('x', encoding='ascii', newline='\n') as f:
            f.write(checksum_text)
    assert json.loads(manifest.read_text(encoding='utf-8')) == expected, 'manifest differs from verified ZIPs'
    assert sums.read_text(encoding='ascii') == checksum_text, 'SHA256SUMS differs from verified ZIPs'


if __name__ == '__main__':
    parser = argparse.ArgumentParser(__doc__)
    parser.add_argument('directory', type=Path)
    parser.add_argument('--repo', type=Path, default=REPOSITORY, help='local Git repository containing the exact candidate objects')
    parser.add_argument('--commit', required=True)
    parser.add_argument('--version', required=True)
    parser.add_argument('--native', action='store_true')
    parser.add_argument('--write-manifest', action='store_true', help='create manifest and SHA256SUMS from verified data; otherwise check existing files')
    parser.add_argument('--expected-sha256', action='append', metavar='FILENAME=HEX', help='repeat for both ZIPs; external Owner/Release identity anchor')
    args = parser.parse_args()
    digests = None
    if args.expected_sha256:
        digests = dict(value.split('=', 1) for value in args.expected_sha256)
        assert len(args.expected_sha256) == len(digests), 'duplicate external digest'
        assert all(re.fullmatch('[0-9a-f]{64}', value) for value in digests.values())
    results = verify(args.directory, args.commit, args.version, args.native, args.repo, digests)
    metadata(args.directory, results, args.write_manifest)
    print(json.dumps(results, indent=2))
