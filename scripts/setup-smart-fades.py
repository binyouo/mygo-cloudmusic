#!/usr/bin/env python3
"""Prepare a self-contained local inference runtime; never modifies system Python."""
import hashlib
import json
from pathlib import Path
import subprocess
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
SOURCE = ROOT / 'scripts/smart_fades'
PYTHON = ROOT / 'build/smart-fades-python/cpython-3.12.14-macos-aarch64-none/bin/python3'


def main():
    subprocess.run(['uv', 'python', 'install', '3.12.14', '--no-bin', '--install-dir', str(ROOT / 'build/smart-fades-python')], check=True)
    subprocess.run(['uv', 'pip', 'install', '--python', str(PYTHON), '--target', str(PYTHON.parent.parent / 'lib/python3.12/site-packages'), '--link-mode', 'copy', '-r', str(SOURCE / 'requirements.lock')], check=True)
    models = ROOT / 'build/smart-fades-models'
    models.mkdir(parents=True, exist_ok=True)
    for name, info in json.loads((SOURCE / 'models.json').read_text()).items():
        path = models / name
        if path.is_file() and hashlib.sha256(path.read_bytes()).hexdigest() == info['sha256']:
            continue
        temporary = path.with_suffix(path.suffix + '.part')
        try:
            with urllib.request.urlopen(info['url'], timeout=120) as response, temporary.open('wb') as dest:
                while chunk := response.read(1024 * 1024):
                    dest.write(chunk)
            if hashlib.sha256(temporary.read_bytes()).hexdigest() != info['sha256']:
                raise RuntimeError('Model checksum mismatch: ' + name)
            temporary.replace(path)
        finally:
            temporary.unlink(missing_ok=True)
    subprocess.run([str(PYTHON), str(SOURCE / 'runner.py'), 'check'], check=True)


if __name__ == '__main__':
    main()
