"""Extract this M0's single-file VPK v1 into a new, empty directory.

Sequential IO, bounded size, path validation and CRC checks; never overwrites.
"""
import argparse
import binascii
import hashlib
import json
from pathlib import Path, PurePosixPath
import struct

EXPECTED = '27b4b93824c2a5ebc96ad9986954e9e6602981800085dee3805739afb4249c0c'


def extract(source, target):
    with source.open('rb') as f:
        digest = hashlib.file_digest(f, 'sha256').hexdigest()
        if digest != EXPECTED:
            raise ValueError('Not the user-provided M0 VPK')
        f.seek(0)
        magic, version, size = struct.unpack('<III', f.read(12))
        if (magic, version) != (0x55AA1234, 1) or size > 16 * 1024 * 1024:
            raise ValueError('Unsupported VPK header')
        tree = f.read(size)
        cursor = 0

        def string():
            nonlocal cursor
            end = tree.index(b'\0', cursor)
            value = tree[cursor:end].decode('utf-8')
            cursor = end + 1
            return value

        entries, seen, total = [], set(), 0
        while (ext := string()):
            while (folder := string()):
                while (name := string()):
                    crc, preload, archive, offset, length, terminator = struct.unpack_from('<IHHIIH', tree, cursor)
                    cursor += 18
                    prefix = tree[cursor:cursor + preload]
                    cursor += preload
                    path = PurePosixPath(('' if folder == ' ' else folder + '/') + name + ('' if ext == ' ' else '.' + ext))
                    if path.is_absolute() or '..' in path.parts or '\\' in str(path) or ':' in str(path) or str(path) in seen:
                        raise ValueError('Unsafe or duplicate path')
                    if archive != 0x7FFF or terminator != 0xFFFF or len(prefix) != preload or 12 + size + offset + length > source.stat().st_size:
                        raise ValueError('Invalid or external archive entry')
                    seen.add(str(path))
                    total += preload + length
                    if total > 2 * 1024**3 or len(entries) >= 100000:
                        raise ValueError('Extraction exceeds M0 limits')
                    entries.append((path, crc, prefix, offset, length))
        target.mkdir(parents=False, exist_ok=False)
        for path, crc, prefix, offset, length in entries:
            dest = target.joinpath(*path.parts)
            dest.parent.mkdir(parents=True, exist_ok=True)
            f.seek(12 + size + offset)
            checksum = binascii.crc32(prefix)
            with dest.open('xb') as out:
                out.write(prefix)
                while length:
                    block = f.read(min(length, 1024 * 1024))
                    if not block:
                        raise ValueError('Truncated payload')
                    out.write(block)
                    checksum = binascii.crc32(block, checksum)
                    length -= len(block)
            if checksum != crc:
                raise ValueError('CRC mismatch: ' + str(path))
        return {'sourceSHA256': digest, 'files': len(entries), 'bytes': total, 'crcVerified': True}


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('source', type=Path)
    parser.add_argument('target', type=Path)
    args = parser.parse_args()
    print(json.dumps(extract(args.source, args.target)))
