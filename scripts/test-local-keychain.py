#!/usr/bin/env python3
"""Exercise local Keychain access across different signed callers, with UI disabled."""
import hashlib
import importlib.util
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import uuid

root = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('local_build', root / 'scripts/build-local-macos.py')
build = importlib.util.module_from_spec(spec)
spec.loader.exec_module(build)

CALLER = r'''
#include <stdio.h>
#include <unistd.h>
#include <sys/wait.h>
int main(int argc, char **argv) {
    if (argc != 3) return 2;
    fprintf(stderr, "caller build: %s\n", BUILD);
    pid_t child = fork();
    if (child < 0) return 2;
    if (child == 0) {
        execl(argv[1], argv[1], argv[2], "--no-ui", NULL);
        _exit(2);
    }
    int status;
    if (waitpid(child, &status, 0) < 0 || !WIFEXITED(status)) return 2;
    return WEXITSTATUS(status);
}
'''


def main():
    identity = os.environ.get('MYGO_SIGNING_IDENTITY', 'MyGo CloudMusic Local Signing')
    identifier = 'dev.egoist.cloudmusic.keychain-test'
    service = identifier + '.' + str(uuid.uuid4())
    with tempfile.TemporaryDirectory(prefix='mygo-keychain-test-') as temporary:
        folder = Path(temporary)
        helper = folder / 'MyGo Keychain'
        build.build_helper(root, helper, identity, identifier, service)
        original_hash = hashlib.sha256(helper.read_bytes()).digest()
        (folder / 'caller.c').write_text(CALLER)
        for variant, signer, name in [('A', identity, identifier), ('B', identity, identifier),
                                      ('wrong-id', identity, identifier + '.other'),
                                      ('adhoc', '-', identifier)]:
            caller = folder / variant
            build.run('clang', '-O2', '-DBUILD="' + variant + '"', str(folder / 'caller.c'), '-o', str(caller))
            build.run('codesign', '--force', '--sign', signer, '--timestamp=none',
                      '--options', 'runtime', '--identifier', name, str(caller))

        def call(variant, op, value=None, target=helper):
            return subprocess.run([str(folder / variant), str(target), op], input=value, capture_output=True)

        def succeeded(result):
            assert result.returncode == 0, result.stderr.decode()
            return result.stdout

        fixture = b'disposable-keychain-test-cookie'
        try:
            # Rebuilding the caller used to change its partition and trigger a prompt.
            succeeded(call('A', 'write', fixture))
            assert hashlib.sha256((folder / 'A').read_bytes()).digest() != hashlib.sha256((folder / 'B').read_bytes()).digest()
            assert succeeded(call('B', 'read')) == fixture
            succeeded(call('B', 'write', fixture + b'-updated'))
            assert succeeded(call('A', 'read')) == fixture + b'-updated'
            installed = folder / 'installed' / helper.name
            installed.parent.mkdir()
            shutil.copy2(helper, installed)
            assert succeeded(call('B', 'read', target=installed)) == fixture + b'-updated'
            # Neither another app ID nor an ad-hoc copy may use the helper.
            for variant in ['wrong-id', 'adhoc']:
                denied = call(variant, 'read')
                assert denied.returncode != 0 and not denied.stdout and b'caller rejected:' in denied.stderr
            denied = subprocess.run([str(helper), 'read', '--no-ui'], capture_output=True)
            assert denied.returncode != 0 and not denied.stdout and b'caller rejected:' in denied.stderr
            assert call('B', 'write', b'x' * 2561).returncode != 0
            assert succeeded(call('A', 'read')) == fixture + b'-updated'
            succeeded(call('B', 'delete'))
            missing = call('A', 'read')
            assert missing.returncode != 0 and b'-25300' in missing.stderr
            assert hashlib.sha256(helper.read_bytes()).digest() == original_hash
        finally:
            succeeded(call('A', 'delete'))
    print('PASS: different builds and paths share Keychain access without UI; unauthorized callers rejected; fixture removed.')


if __name__ == '__main__':
    main()
