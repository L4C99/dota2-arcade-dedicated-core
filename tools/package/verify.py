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
MARKER = 'SV:  Connection to Steam servers successful.'


def verify(directory, commit, version, native=False):
    results = []
    for target in ('windows', 'linux'):
        suffix = '.exe' if target == 'windows' else ''
        path = directory / f'd2core-v{version}-{target}-amd64.zip'
        digest = hashlib.sha256(path.read_bytes()).hexdigest()
        expected = path.with_suffix('.zip.sha256').read_text().split()
        assert expected == [digest, path.name], f'checksum mismatch: {path}'
        with zipfile.ZipFile(path) as z:
            names = z.namelist()
            assert len(names) == len(set(n.lower() for n in names)), 'duplicate archive path'
            assert set(names) == REQUIRED | {'d2core'+suffix, 'launcher-example'+suffix}, names
            assert z.testzip() is None, 'ZIP CRC failure'
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
                    value = json.loads(subprocess.check_output([str(binary), 'version', '--json'], text=True))
                    for key in ('version', 'gitCommit', 'gitDirty', 'buildTime', 'protocolVersion', 'schemaVersion', 'formatVersion'):
                        assert value[key] == build[key], (key, value, build)
                    assert value['goVersion'] == 'go'+build['goVersion']
                    smoke = 'PASS'
            results.append(dict(file=path.name, sha256=digest, bytes=path.stat().st_size,
                                entries=sorted(names), build=build, nativeVersion=smoke))
    return results


if __name__ == '__main__':
    parser = argparse.ArgumentParser(__doc__)
    parser.add_argument('directory', type=Path)
    parser.add_argument('--commit', required=True)
    parser.add_argument('--version', required=True)
    parser.add_argument('--native', action='store_true')
    args = parser.parse_args()
    print(json.dumps(verify(args.directory, args.commit, args.version, args.native), indent=2))
