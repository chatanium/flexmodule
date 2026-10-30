#!/usr/bin/env python3
"""Resolve the required, unreplaced canonical SDK through Go module metadata."""
import argparse
import json
import os
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[1]
MODULE = 'github.com/chatanium/discordgo'


def resolve(go=None, module_dir=ROOT):
    toolchain = next(line.split(':=', 1)[1].strip()
                     for line in (ROOT / 'tools/versions.mk').read_text().splitlines()
                     if line.startswith('GO_TOOLCHAIN :='))
    env = {**os.environ, 'GO_TOOLCHAIN': toolchain,
           'GOTOOLCHAIN': toolchain, 'GOWORK': 'off', 'GOFLAGS': ''}
    # Use the selected compiler's own tool and standard-library root.
    env.pop('GOROOT', None)
    if go is None:
        # Reuse the pinned toolchain selection used by every make interface.
        go = subprocess.check_output(
            ['bash', '-euo', 'pipefail', '-c',
             'source "$1/tools/common.sh"; use_go; printf "%s" "$GO_BINARY"',
             'bash', str(ROOT)], env=env, text=True)
    actual = subprocess.check_output([go, 'env', 'GOVERSION'], env=env, text=True).strip()
    if actual != toolchain:
        raise RuntimeError(f'Go version is {actual}; require {toolchain}')
    metadata = json.loads(subprocess.check_output(
        [go, '-C', str(Path(module_dir).resolve()), 'list', '-mod=readonly', '-m', '-json', MODULE],
        env=env, text=True))
    if metadata.get('Path') != MODULE or metadata.get('Replace'):
        raise RuntimeError(f'{module_dir} must require {MODULE} without a replace directive')
    if not metadata.get('Version'):
        raise RuntimeError(f'{module_dir} must require a published {MODULE} version')
    sums = (Path(module_dir) / 'go.sum').read_text().splitlines()
    expected = next((line.split()[2] for line in sums
                     if line.split()[:2] == [MODULE, metadata['Version']]), None)
    if not expected or metadata.get('Sum', expected) != expected:
        raise RuntimeError(f'{module_dir} must pin the published {MODULE} checksum in go.sum')
    metadata['Sum'] = expected
    if not metadata.get('Dir'):
        # Populate only the module cache. An isolated cwd prevents download
        # from updating the consuming module's go.mod or go.sum.
        with tempfile.TemporaryDirectory(prefix='flexmodule-sdk-') as directory:
            downloaded = json.loads(subprocess.check_output(
                [go, '-C', directory, 'mod', 'download', '-json', MODULE + '@' + metadata['Version']],
                env=env, text=True))
        if any(downloaded.get(key) != metadata[key] for key in ('Path', 'Version', 'Sum')):
            raise RuntimeError('Downloaded SDK does not match the required module metadata')
        metadata['Dir'] = downloaded['Dir']
    if not Path(metadata['Dir']).is_dir():
        raise RuntimeError(f'{MODULE} source directory is unavailable')
    return metadata


def provenance(metadata):
    return {key: metadata[key] for key in ('Path', 'Version', 'Sum')}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--go', help='Selected Go binary (otherwise use tools/common.sh)')
    parser.add_argument('--module-dir', type=Path, default=ROOT)
    parser.add_argument('--json', action='store_true', help='Print Go module metadata instead of the source directory')
    args = parser.parse_args()
    metadata = resolve(args.go, args.module_dir)
    print(json.dumps(metadata) if args.json else metadata['Dir'])


if __name__ == '__main__':
    main()
