"""CI-only read-only metadata diagnostic. No environment or command-line dump."""
import json
import os
from pathlib import Path

uid = os.geteuid()
unreadable = []
for path in Path('/proc').iterdir():
    if not path.name.isdigit():
        continue
    try:
        if path.stat().st_uid != uid:
            continue
        os.readlink(path / 'exe')
    except FileNotFoundError:
        continue
    except PermissionError as error:
        try:
            name = (path / 'comm').read_text().strip()
        except OSError:
            name = 'unreadable'
        unreadable.append({'pid': int(path.name), 'comm': name, 'error': str(error)})
print(json.dumps({'uid': uid, 'sameUserUnreadableExe': unreadable}, indent=2))
