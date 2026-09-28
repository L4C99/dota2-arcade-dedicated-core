"""Offline negative regression tests for the artifact verifier."""
import hashlib
import importlib.util
import json
import tempfile
import unittest
import zipfile
from pathlib import Path
spec = importlib.util.spec_from_file_location('verify', Path(__file__).with_name('verify.py'))
verify = importlib.util.module_from_spec(spec)
spec.loader.exec_module(verify)

class ArtifactTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.directory = Path(self.temp.name)

    def make(self, omit=None, wrong_name=False, bad_identity=False, no_marker=False, duplicate=False):
        for target in ('windows', 'linux'):
            names = verify.REQUIRED | ({'d2core.exe', 'launcher-example.exe'} if target == 'windows' else {'d2core', 'launcher-example'})
            p = self.directory / f'd2core-v0.1.2-rc.1-{target}-amd64.zip'
            with zipfile.ZipFile(p, 'w') as z:
                for name in sorted(names - {omit}):
                    body = b'fixture'
                    if name == 'BUILD.json':
                        body = json.dumps(dict(version='wrong' if bad_identity else '0.1.2-rc.1',gitCommit='a'*40,gitDirty=False,os=target,arch='amd64',protocolVersion=1,schemaVersion=1,formatVersion=2,goVersion='1.27.1')).encode()
                    if name.endswith('.json') and name.startswith('examples/'):
                        body = json.dumps(dict(schemaVersion=1, readiness=dict(successAll=['map','host','script','wrong' if no_marker else verify.MARKER]))).encode()
                    z.writestr('wrong/LICENSE' if wrong_name and name == 'LICENSE' else name, body)
                if duplicate:
                    z.writestr('LICENSE', b'duplicate')
            p.with_suffix('.zip.sha256').write_text(hashlib.sha256(p.read_bytes()).hexdigest()+'  '+p.name+'\n')

    def check(self):
        return verify.verify(self.directory, 'a'*40, '0.1.2-rc.1')

    def test_valid(self):
        self.make()
        self.assertEqual(len(self.check()), 2)

    def test_required(self):
        for required in verify.REQUIRED:
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

if __name__ == '__main__':
    unittest.main()
