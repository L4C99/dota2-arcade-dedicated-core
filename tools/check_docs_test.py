"""文档状态门槛的正反例；不依赖游戏或第三方 Python 包。"""

import unittest
from check_docs import check_document, check_development_marker, LIVING_DOCS

CHINESE = "这是文档检查的中文正文，用于确认中文语言门槛与版本状态规则彼此独立。\n"


class DocumentationChecks(unittest.TestCase):
    def test_source_development_marker(self):
        self.assertFalse(check_development_marker('var releaseVersion = "0.1.2-dev"', "0.1.2"))
        for marker in ("0.1.1-dev", "0.1.3-dev", "0.1.2"):
            with self.subTest(marker=marker):
                self.assertTrue(check_development_marker(f'var releaseVersion = "{marker}"', "0.1.2"))
        self.assertTrue(check_development_marker("", "0.1.2"))

    def check(self, text, name="README.md", version="0.1.2"):
        return check_document(name, CHINESE + text, version)

    def test_stale_current_declarations(self):
        for label in ("当前稳定版本为", "正式稳定版本：", "当前正式版本是", "当前版本=", "current stable is"):
            with self.subTest(label=label):
                self.assertTrue(self.check(label + " **v0.1.1**"))
                self.assertFalse(self.check(label + " [v0.1.2](https://example.com)"))

    def test_historical_references_allowed(self):
        self.assertFalse(self.check("v0.1.2 沿用 v0.1.1 Runtime 行为。\nv0.1.1 历史 Release\n"
                                    "[历史验证](blob/v0.1.1/docs/validation/m0.md)\nv0.1.1 tag/assets 未修改"))
        self.assertFalse(self.check("历史 v0.1.1 当时尚未创建 tag。当前稳定版本为 v0.1.2。"))

    def test_stable_residue(self):
        for phrase in ("unreleased", "未发布", "current candidate", "当前候选", "release preparation", "尚未创建 tag", "尚未创建 Release"):
            with self.subTest(phrase=phrase):
                self.assertTrue(self.check("v0.1.2 " + phrase))
                self.assertTrue(self.check("当前状态：" + phrase))
                self.assertTrue(self.check(phrase, name="RELEASE_NOTES.md"))
        self.assertFalse(self.check("当前候选 v0.1.3-rc.1 未发布", version="0.1.3-rc.1"))

    def test_current_candidate_without_version(self):
        self.assertTrue(self.check("当前候选等待发布"))
        self.assertTrue(self.check("current candidate awaiting review"))

    def test_language_and_code_exclusions(self):
        self.assertTrue(check_document("RELEASE_NOTES.md", "# Release\nEnglish only.\n", "0.1.2"))
        self.assertTrue(check_document("README.md", "# English\n```text\n" + CHINESE + "```", "0.1.2"))
        self.assertFalse(self.check("```text\n当前版本 v0.1.1 未发布\n```\n> 当前版本 v0.1.1 未发布"))

    def test_historical_and_legal_scope(self):
        self.assertNotIn("LICENSE", LIVING_DOCS)
        self.assertNotIn("LICENSING.md", LIVING_DOCS)
        self.assertEqual([p for p in LIVING_DOCS if p.startswith("docs/validation/")], ["docs/validation/README.md"])


if __name__ == "__main__":
    unittest.main()
