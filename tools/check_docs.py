"""源码仓库文档门槛；仅检查明确列出的现行文档，不改写文件。"""

from pathlib import Path
import re
import sys

ROOT = Path(__file__).resolve().parents[1]
LIVING_DOCS = (
    "README.md", "CHANGELOG.md", "RELEASE_NOTES.md",
    "docs/delivery.md", "docs/development.md", "docs/operations.md",
    "docs/local-api.md", "docs/a2s.md", "docs/roadmap.md",
    "docs/documentation-policy.md", "docs/validation/README.md",
    "examples/README.md", "examples/launcher/README.md",
    "client/README.md", "tools/README.md",
)
VERSION = r"v?(\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?)"
CURRENT = r"(?:当前稳定版本|正式稳定版本|当前正式版本|当前版本|current\s+stable(?:\s+version)?)"
DECLARATION = re.compile(CURRENT + r"\s*(?:为|是|：|:|=|is)?\s*" + VERSION, re.I)
RESIDUE = re.compile(r"unreleased|未发布|current\s+candidate|当前候选|release\s+preparation|尚未创建\s*(?:tag|Release)", re.I)


def prose_lines(text):
    """跳过代码块和引用；保留行号，去掉链接目标而保留标签。"""
    fence = None
    for number, line in enumerate(text.splitlines(), 1):
        stripped = line.lstrip()
        marker = re.match(r"(`{3,}|~{3,})", stripped)
        if marker:
            token = marker.group(1)
            if fence is None:
                fence = token
            elif token[0] == fence[0] and len(token) >= len(fence):
                fence = None
            continue
        if fence or stripped.startswith(">"):
            continue
        line = re.sub(r"\[([^\]]+)\]\([^)]*\)", r"\1", line)
        yield number, line.replace("`", "").replace("*", "")


def check_document(name, text, version):
    lines = list(prose_lines(text))
    errors = []
    # 这是低成本防退化检查，不能代替中文措辞与技术语义的人工审阅。
    if len(re.findall(r"[\u4e00-\u9fff]", "\n".join(s for _, s in lines))) < 20:
        errors.append(f"{name}: 正文中文不足 20 字；现行文档应以简体中文为主")
    stable = "-" not in version
    for number, line in lines:
        for match in DECLARATION.finditer(line):
            if match.group(1) != version:
                errors.append(f"{name}:{number}: 当前版本 {match.group(1)} 与 VERSION={version} 不符")
        # 按分句判定，避免同句中合法的历史引用被当前状态词牵连。
        for clause in re.split(r"[。；;]|(?<=[.!?])\s+", line):
            current_context = re.search(CURRENT + r"|当前状态|本次发布|本版本|当前候选|current\s+candidate", clause, re.I)
            version_context = re.search(r"(?<![\w.])v?" + re.escape(version) + r"(?![\w.-])", clause)
            # Release Notes 开篇/标题默认描述该次发布；历史章节以旧版本明确标识。
            release_context = name == "RELEASE_NOTES.md" and not re.search(VERSION, clause)
            if stable and (current_context or version_context or release_context) and RESIDUE.search(clause):
                errors.append(f"{name}:{number}: 正式版本仍有候选/未发布状态措辞")
    return errors


def check_development_marker(source, version):
    """源码开发标识跟随发布版本基号，不改变正式包的版本注入。"""
    expected = version.split("-", 1)[0] + "-dev"
    markers = re.findall(r'^var releaseVersion = "([^"]+)"', source, re.M)
    if markers != [expected]:
        return [f"cmd/d2core/main.go: 源码开发标识须为 {expected}，实际为 {markers}"]
    return []


def check_repo(root):
    version = (root / "tools/package/VERSION").read_text(encoding="utf-8").strip()
    if not re.fullmatch(VERSION, version):
        return ["tools/package/VERSION: 无效版本"]
    errors = check_development_marker(
        (root / "cmd/d2core/main.go").read_text(encoding="utf-8"), version)
    for name in LIVING_DOCS:
        path = root / name
        if not path.is_file():
            errors.append(f"{name}: 必需现行文档缺失")
        else:
            errors.extend(check_document(name, path.read_text(encoding="utf-8"), version))
    return errors


if __name__ == "__main__":
    problems = check_repo(ROOT)
    print("\n".join(problems) if problems else "PASS: documentation language and release-state checks")
    sys.exit(bool(problems))
