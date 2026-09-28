"""Offline packaging contract, Git provenance and metadata regressions."""
import hashlib
import importlib.util
import json
import subprocess
import tempfile
import unittest
import warnings
import zipfile
from pathlib import Path

spec = importlib.util.spec_from_file_location('verify', Path(__file__).with_name('verify.py'))
verify = importlib.util.module_from_spec(spec)
spec.loader.exec_module(verify)

# Deliberately fixed acceptance floor, independent of both production lists.
# Removing an item from builder AND verifier must still fail this test.
FROZEN_REQUIRED = set('''LICENSE LICENSING.md THIRD_PARTY_NOTICES.md CHANGELOG.md
README.md RELEASE_NOTES.md BUILD.json docs/delivery.md docs/operations.md
 docs/local-api.md docs/a2s.md examples/README.md examples/template.windows.json
examples/template.linux.json examples/launcher/README.md examples/launcher/main.go'''.split())
VERSION = (Path(__file__).with_name('VERSION')).read_text().strip()


class ArtifactTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.scratch = tempfile.TemporaryDirectory()
        cls.repo = Path(cls.scratch.name)
        for name in FROZEN_REQUIRED - {'BUILD.json'}:
            p = cls.repo / name
            p.parent.mkdir(parents=True, exist_ok=True)
            data = b'fixture\n'
            if name.startswith('examples/template.'):
                data = json.dumps(dict(schemaVersion=1, readiness=dict(successAll=['map','host','script',verify.MARKER]))).encode()
            p.write_bytes(data)
        (cls.repo/'tools/package').mkdir(parents=True)
        (cls.repo/'tools/package/main.go').write_bytes(Path(__file__).with_name('main.go').read_bytes())
        (cls.repo/'tools/package/VERSION').write_bytes((VERSION+'\n').encode())
        def git(*args):
            return subprocess.check_output(['git','-C',str(cls.repo),*args], stderr=subprocess.PIPE)
        git('init','-q')
        git('config','core.autocrlf','false')
        git('config','user.name','Package test')
        git('config','user.email','package@example.invalid')
        git('config','commit.gpgsign','false')
        git('add','.')
        git('commit','-qm','fixture')
        cls.commit = git('rev-parse','HEAD').decode().strip()

    @classmethod
    def tearDownClass(cls):
        cls.scratch.cleanup()

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.directory = Path(self.temp.name)

    def make(self, omit=None, wrong_name=False, bad_identity=False, no_marker=False, duplicate=False, transcribe=False):
        for target in ('windows', 'linux'):
            names = FROZEN_REQUIRED | ({'d2core.exe', 'launcher-example.exe'} if target == 'windows' else {'d2core', 'launcher-example'})
            p = self.directory / f'd2core-v{VERSION}-{target}-amd64.zip'
            with zipfile.ZipFile(p, 'w') as z:
                for name in sorted(names - {omit}):
                    body = (self.repo/name).read_bytes() if name in FROZEN_REQUIRED-{'BUILD.json'} else b'binary fixture'
                    if name == 'BUILD.json':
                        body = json.dumps(dict(version='wrong' if bad_identity else VERSION,gitCommit=self.commit,gitDirty=False,os=target,arch='amd64',protocolVersion=1,schemaVersion=1,formatVersion=2,goVersion='1.27.1',buildTime='2026-09-29T00:00:00Z')).encode()
                    if name.startswith('examples/template.') and no_marker:
                        body = body.replace(verify.MARKER.encode(), b'wrong')
                    if name == 'README.md' and transcribe:
                        body = body.replace(b'\n', b'\r\n')
                    z.writestr('wrong/LICENSE' if wrong_name and name == 'LICENSE' else name, body)
                if duplicate:
                    with warnings.catch_warnings():
                        warnings.simplefilter('ignore', UserWarning)
                        z.writestr('LICENSE', b'duplicate')
            p.with_suffix('.zip.sha256').write_text(hashlib.sha256(p.read_bytes()).hexdigest()+'  '+p.name+'\n')

    def check(self, **kwargs):
        return verify.verify(self.directory, self.commit, VERSION, repo=self.repo, **kwargs)

    def test_contract_floor_and_equality(self):
        self.assertEqual(verify.REQUIRED, FROZEN_REQUIRED)
        verify.check_contract(self.repo, self.commit)

    def test_valid_git_blob_entries(self):
        self.make()
        self.assertEqual(len(self.check()), 2)

    def test_required(self):
        for required in FROZEN_REQUIRED:
            with self.subTest(required=required):
                self.make(omit=required)
                with self.assertRaises(AssertionError): self.check()

    def test_corruption(self):
        for kwargs in [dict(wrong_name=True), dict(bad_identity=True), dict(no_marker=True), dict(duplicate=True)]:
            with self.subTest(kwargs=kwargs):
                self.make(**kwargs)
                with self.assertRaises(AssertionError): self.check()
        self.make()
        next(self.directory.glob('*.sha256')).write_text('0'*64+'  wrong.zip')
        with self.assertRaises(AssertionError): self.check()

    def test_transcribed_zip_with_consistent_checksum_fails(self):
        self.make(transcribe=True)
        with self.assertRaisesRegex(AssertionError, 'provenance mismatch: README.md'):
            self.check()

    def test_external_anchor(self):
        self.make()
        digests = {p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in self.directory.glob('*.zip')}
        self.check(expected_sha256=digests)
        digests[next(iter(digests))] = '0'*64
        with self.assertRaisesRegex(AssertionError,'external digest mismatch'):
            self.check(expected_sha256=digests)

    def test_version_anchor(self):
        self.make()
        with self.assertRaisesRegex(AssertionError, 'VERSION'):
            verify.verify(self.directory, self.commit, '9.9.9', repo=self.repo)

    def test_manifest_generated_and_drift_rejected(self):
        self.make()
        results = self.check()
        verify.metadata(self.directory, results, write=True)
        verify.metadata(self.directory, results)
        path = self.directory/'RELEASE-MANIFEST.json'
        original = path.read_text()
        for key, bad in [('version','wrong'), ('gitCommit','0'*40), ('buildTime','wrong')]:
            data = json.loads(original); data[key] = bad
            path.write_text(json.dumps(data))
            with self.assertRaisesRegex(AssertionError,'manifest'): verify.metadata(self.directory, results)
        for key, bad in [('file','wrong'),('bytes',0),('sha256','0'*64),('entries',[]),('build',{})]:
            data = json.loads(original); data['artifacts'][0][key] = bad
            path.write_text(json.dumps(data))
            with self.assertRaisesRegex(AssertionError,'manifest'): verify.metadata(self.directory, results)
        path.write_text(original)
        (self.directory/'SHA256SUMS').write_text('wrong')
        with self.assertRaisesRegex(AssertionError,'SHA256SUMS'): verify.metadata(self.directory, results)

if __name__ == '__main__':
    unittest.main()
