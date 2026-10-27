#!/usr/bin/env python3
"""Build the local macOS app with the same Keychain identity on every rebuild."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import ssl
import subprocess
import tempfile


def run(*args, **kwargs):
    return subprocess.run(args, check=True, **kwargs)


def signing_requirement(identity, identifier):
    pem = subprocess.check_output(['security', 'find-certificate', '-c', identity, '-p'], text=True)
    fingerprint = hashlib.sha1(ssl.PEM_cert_to_DER_cert(pem)).hexdigest()
    return f'identifier {json.dumps(identifier)} and certificate leaf = H"{fingerprint}"'


def build_helper(root, target, identity, identifier, service):
    requirement = signing_requirement(identity, identifier)
    run('clang', '-O2', '-Wall', '-Wextra', '-Werror', '-Wno-deprecated-declarations',
        '-mmacosx-version-min=13.0',
        '-DAPP_REQUIREMENT=' + json.dumps(requirement),
        '-DKEYCHAIN_SERVICE=' + json.dumps(service),
        str(root / 'scripts/local-keychain.c'), '-framework', 'Security',
        '-framework', 'CoreFoundation', '-o', str(target))
    run('codesign', '--force', '--sign', identity, '--timestamp=none',
        '--options', 'runtime', '--identifier', identifier + '.keychain', str(target))


def cached_helper(root, identity, identifier, arch):
    requirement = signing_requirement(identity, identifier)
    recipe = (root / 'scripts/local-keychain.c').read_bytes() + (requirement + arch).encode()
    digest = hashlib.sha256(recipe).hexdigest()
    cache = Path.home() / 'Library/Application Support' / identifier / 'local-signing' / digest
    cache.mkdir(mode=0o700, parents=True, exist_ok=True)
    helper = cache / 'MyGo Keychain'
    if not helper.exists():
        with tempfile.TemporaryDirectory(dir=cache) as staging:
            built = Path(staging) / helper.name
            build_helper(root, built, identity, identifier, identifier)
            built.rename(helper)
    run('codesign', '--verify', '--strict', '-R',
        '=' + signing_requirement(identity, identifier + '.keychain'), str(helper))
    return helper


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.parse_args()
    root = Path(__file__).resolve().parents[1]
    config = json.loads((root / 'mygo.json').read_text())
    local_name = 'MyGo云音乐'
    local_identifier = config['identifier'] + '.local'
    identity = os.environ.get('MYGO_SIGNING_IDENTITY', 'MyGo CloudMusic Local Signing')
    # The certificate authenticates the caller; the unchanged helper keeps
    # the Keychain partition identity, which a self-signed certificate cannot.
    run('security', 'find-certificate', '-c', identity, stdout=subprocess.DEVNULL)
    arch = subprocess.check_output(['go', 'env', 'GOARCH'], text=True).strip()
    output = root / config.get('out', 'build') / f'darwin-{arch}'
    target = output / (local_name + '.app')
    previous = output / (config['name'] + '.app')
    resources = target if target.is_dir() else previous
    if not resources.is_dir():
        parser.error(f'An existing macOS bundle is needed for its icon and resources: {resources}')
    helper = cached_helper(root, identity, local_identifier, arch)
    with tempfile.TemporaryDirectory(prefix='.local-build-', dir=target.parent) as temporary:
        bundle = Path(temporary) / target.name
        run('ditto', str(resources), str(bundle))
        # Reuse the bundle resources so local builds do not need Xcode's actool.
        plist = bundle / 'Contents/Info.plist'
        import plistlib
        info = plistlib.loads(plist.read_bytes())
        for key, value in {
            'CFBundleName': local_name, 'CFBundleDisplayName': local_name,
            'CFBundleIdentifier': local_identifier,
            'CFBundleShortVersionString': config['version'],
            'CFBundleVersion': config['version'],
        }.items():
            info[key] = value
        plist.write_bytes(plistlib.dumps(info))
        flags = ['-s', '-w', '-X', 'netease-music-mygo/internal/app.localBuild=1']
        for key, value in {
            'packageName': local_name, 'packageVersion': config['version'],
            'packageIdentifier': local_identifier, 'production': '1',
        }.items():
            if "'" in value or '\n' in value:
                parser.error(f'Unsupported linker value for {key}')
            flags.extend(['-X', f"'github.com/egoist/mygo.{key}={value}'"])
        env = dict(os.environ, CGO_ENABLED='0', GOOS='darwin')
        run('go', 'build', '-trimpath', '-tags', 'mygo_noinspector,mygo_local_keychain', '-ldflags',
            ' '.join(flags), '-o', str(bundle / 'Contents/MacOS' / info['CFBundleExecutable']),
            '.', cwd=root, env=env)
        helpers = bundle / 'Contents/Helpers'
        helpers.mkdir(exist_ok=True)
        run('ditto', str(helper), str(helpers / helper.name))
        run('codesign', '--force', '--sign', identity, '--timestamp=none', '--options', 'runtime', str(bundle))
        run('codesign', '--verify', '--deep', '--strict', str(bundle))
        run('codesign', '-d', '-r-', str(bundle))
        if target.exists():
            shutil.rmtree(target)
        bundle.rename(target)
        if previous != target and previous.exists():
            shutil.rmtree(previous)
        print(f'Built: {target}')
        print('Quit any running older copy before opening the updated app.')


if __name__ == '__main__':
    main()
