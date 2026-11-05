#!/usr/bin/env python3
"""Rebuild the existing macOS profile and bundle Smart Fades' native libraries."""
import datetime
import hashlib
import os
from pathlib import Path
import plistlib
import shlex
import shutil
import subprocess
import tempfile


def run(*args, **kwargs):
    return subprocess.run(args, check=True, **kwargs)


def bundle_library(source, directory, copied):
    source = source.resolve()
    if source in copied:
        return copied[source]
    target = directory / source.name
    shutil.copy2(source, target)
    target.chmod(0o755)
    copied[source] = target
    run('install_name_tool', '-id', '@rpath/' + target.name, str(target))
    listing = subprocess.check_output(['otool', '-L', str(source)], text=True)
    for line in listing.splitlines()[1:]:
        dependency = line.strip().split(' (', 1)[0]
        if dependency.startswith(('/System/', '/usr/lib/')):
            continue
        if dependency in ('@rpath/' + source.name, '@loader_path/' + source.name):
            continue
        path = Path(dependency)
        if dependency.startswith(('@loader_path/', '@rpath/')):
            path = source.parent / dependency.split('/', 1)[1]
        if path.is_absolute() and path.resolve() == source:
            continue
        if not path.is_absolute() or not path.exists():
            raise RuntimeError('Cannot resolve native dependency: ' + dependency)
        included = bundle_library(path, directory, copied)
        run('install_name_tool', '-change', dependency,
            '@loader_path/' + included.name, str(target))
    run('codesign', '--force', '--sign', '-', '--timestamp=none', str(target))
    return target


def bundle_analysis(root, bundle):
    """Carry Python, Torch, models and source together, with no Homebrew links."""
    python = root / 'build/smart-fades-python/cpython-3.12.14-macos-aarch64-none'
    source = root / 'scripts/smart_fades'
    models = root / 'build/smart-fades-models'
    if not (python / 'bin/python3').is_file():
        raise RuntimeError('Run python3 scripts/setup-smart-fades.py first')
    import json
    for name, info in json.loads((source / 'models.json').read_text()).items():
        path = models / name
        if not path.is_file() or hashlib.sha256(path.read_bytes()).hexdigest() != info['sha256']:
            raise RuntimeError('Missing or mismatched model: ' + name)
    dest = bundle / 'Contents/Resources/smart-fades'
    if dest.exists():
        shutil.rmtree(dest)
    shutil.copytree(source, dest, ignore=shutil.ignore_patterns('__pycache__', '*.pyc', '.gitignore'))
    shutil.copytree(python, dest / 'python', symlinks=True, ignore=shutil.ignore_patterns('__pycache__', '*.pyc'))
    shutil.copytree(models, dest / 'models', ignore=shutil.ignore_patterns('*.part'))
    # Wheels and the standalone interpreter contain nested Mach-O binaries.
    # Sign leaves before their containing app; do not re-sign unrelated assets.
    magics = {b'\xcf\xfa\xed\xfe', b'\xce\xfa\xed\xfe', b'\xfe\xed\xfa\xcf', b'\xca\xfe\xba\xbe'}
    for path in sorted((dest / 'python').rglob('*')):
        if path.is_file() and not path.is_symlink():
            with path.open('rb') as file:
                native = file.read(4) in magics
            if native:
                # Standalone Python and some wheels retain build-machine IDs.
                # Their consumers already use relative paths; make the IDs
                # relocatable too, before sealing/signing the binary.
                if path.suffix == '.dylib':
                    ids = subprocess.check_output(['otool', '-D', str(path)], text=True).splitlines()[1:]
                    if any(item.strip().startswith('/') for item in ids):
                        run('install_name_tool', '-id', '@rpath/' + path.name, str(path), stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
                run('codesign', '--force', '--sign', '-', '--timestamp=none', str(path), stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    run(str(dest / 'python/bin/python3'), str(dest / 'runner.py'), 'check',
        env=dict(os.environ, PYTHONNOUSERSITE='1', PYTHONDONTWRITEBYTECODE='1'))
    run(str(dest / 'python/bin/python3'), str(dest / 'test_runner.py'),
        env=dict(os.environ, PYTHONNOUSERSITE='1', PYTHONDONTWRITEBYTECODE='1'))
    (dest / 'test_runner.py').unlink()


def main():
    root = Path(__file__).resolve().parents[1]
    target = root / 'build/darwin-arm64/MyGo 云音乐.app'
    if not target.is_dir():
        raise RuntimeError('Existing bundle required: ' + str(target))
    info = plistlib.loads((target / 'Contents/Info.plist').read_bytes())
    executable = info['CFBundleExecutable']
    identifier = info['CFBundleIdentifier']
    version = info.get('CFBundleShortVersionString', '0.1.24')
    candidates = [
        Path('/opt/homebrew/opt/rubberband/lib/librubberband.dylib'),
        Path('/usr/local/opt/rubberband/lib/librubberband.dylib'),
        target / 'Contents/Frameworks/librubberband.3.dylib',
        target / 'Contents/Frameworks/librubberband.2.dylib',
    ]
    source = next((path for path in candidates if path.is_file()), None)
    if source is None:
        raise RuntimeError('Install rubberband before building Smart Fades')
    with tempfile.TemporaryDirectory(prefix='.smart-fades-build-', dir=target.parent) as temporary:
        bundle = Path(temporary) / target.name
        shutil.copytree(target, bundle, symlinks=True)
        frameworks = bundle / 'Contents/Frameworks'
        frameworks.mkdir(exist_ok=True)
        bundle_library(source, frameworks, {})
        bundle_analysis(root, bundle)
        licenses = bundle / 'Contents/Resources/licenses'
        licenses.mkdir(parents=True, exist_ok=True)
        for label, path in {
            'RubberBand-COPYING.txt': source.resolve().parents[1] / 'COPYING',
            'libsamplerate-COPYING.txt': Path('/opt/homebrew/opt/libsamplerate/share/doc/libsamplerate/COPYING'),
        }.items():
            if path.is_file():
                shutil.copy2(path, licenses / label)
        notice = root / 'docs/music-assistant-smart-fades.md'
        shutil.copy2(notice, licenses / 'Smart-Fades-implementation.md')
        for path in (root / 'docs/audio-licenses').glob('*.txt'):
            shutil.copy2(path, licenses / path.name)
        flags = shlex.join(['-s', '-w',
                           '-X', 'github.com/egoist/mygo.packageName=' + info.get('CFBundleDisplayName', executable),
                           '-X', 'github.com/egoist/mygo.packageVersion=' + version,
                           '-X', 'github.com/egoist/mygo.packageIdentifier=' + identifier,
                           '-X', 'github.com/egoist/mygo.production=1'])
        binary = bundle / 'Contents/MacOS' / executable
        run('go', 'build', '-trimpath', '-tags', 'mygo_noinspector', '-ldflags', flags,
            '-o', str(binary), '.', cwd=root,
            env=dict(os.environ, CGO_ENABLED='0', GOOS='darwin', GOARCH='arm64'))
        run('codesign', '--force', '--sign', '-', '--timestamp=none', str(bundle))
        run('codesign', '--verify', '--deep', '--strict', str(bundle))
        stamp = datetime.datetime.now().strftime('%Y%m%d-%H%M%S')
        backup = target.with_name('MyGo 云音乐.pre-smart-fades-' + stamp + '.app')
        target.rename(backup)
        bundle.rename(target)
        print('Installed:', target)
        print('Backup:', backup)
        print('SHA256:', hashlib.sha256((target / 'Contents/MacOS' / executable).read_bytes()).hexdigest())


if __name__ == '__main__':
    main()
