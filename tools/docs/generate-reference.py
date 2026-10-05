#!/usr/bin/env python3
"""Generate the user guide index and source navigation; --check refuses drift."""
import argparse
from pathlib import Path
import re
import sys

ROOT = Path(__file__).resolve().parents[2]
USER = ROOT / 'docs/user'
OUTPUTS = {'README.md', 'source-reference.md'}


def title(path):
    match = re.search(r'^#\s+(.+)$', path.read_text(), re.MULTILINE)
    if not match:
        raise ValueError(f'missing page title: {path.relative_to(ROOT)}')
    return match.group(1).replace('|', '\\|')


def link(label, path):
    from os.path import relpath
    return f'[{label}]({relpath(path, USER)})'


def render():
    pages = sorted(p for p in USER.rglob('*.md') if p.name not in OUTPUTS)
    lines = ['# NGFW user guide', '',
             'Generated from the checked-in feature guides. Regenerate with '
             '`python3 tools/docs/generate-reference.py`; verify with `--check`.', '',
             'Start with [First-boot setup](getting-started.md) and the '
             '[CLI reference](cli/reference.md). Feature pages describe configuration, '
             'API/CLI equivalents and feature-specific limits.', '',
             'A guide or source file is not evidence that a feature passed live acceptance. '
             'See the [acceptance register](../status/DEFERRED-ACCEPTANCE.md) and '
             '[product boundaries](../status/have-not.md). For authenticated operations '
             'and request schemas, use the OpenAPI document supplied by your installed API.', '',
             '[Source navigation](source-reference.md) identifies the current API controllers '
             'and schema sources for developers and operators auditing their installed version.', '']
    groups = sorted({p.relative_to(USER).parts[0] for p in pages if p.parent != USER})
    for group in groups:
        lines += [f'## {group.replace("-", " ").title()}', '']
        lines += [f'- {link(title(p), p)}' for p in pages if p.relative_to(USER).parts[0] == group]
        lines.append('')
    ref = ['# API and schema source navigation', '',
           'Generated from source paths; these links describe repository coverage, '
           'not accepted appliance behavior. They do not infer routes from TypeScript '
           'decorators or replace the installed OpenAPI document.', '', '## API controllers', '']
    controllers = sorted(p for p in (ROOT / 'apps/api/src').rglob('*.ts')
                         if (p.name.endswith('.controller.ts') or p.name == 'controller.ts')
                         and not p.name.endswith('.test.ts'))
    ref += [f'- {link(str(p.relative_to(ROOT / "apps/api/src")), p)}' for p in controllers]
    ref += ['', '## Schema sources', '',
            'The root schema defines the configuration contract. Use these sources '
            'to trace validation; sensitive fields remain write-only or redacted as '
            'specified by the individual feature guide.', '']
    schemas = sorted((ROOT / 'packages/schema/src').rglob('*.ts'))
    schemas = [p for p in schemas if not p.name.endswith(('.test.ts', '.spec.ts'))]
    ref += [f'- {link(str(p.relative_to(ROOT / "packages/schema/src")), p)}' for p in schemas]
    return {'README.md': '\n'.join(lines), 'source-reference.md': '\n'.join(ref) + '\n'}, len(pages), len(controllers), len(schemas)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--check', action='store_true')
    args = parser.parse_args()
    outputs, pages, controllers, schemas = render()
    for name, body in outputs.items():
        path = USER / name
        if args.check:
            if not path.exists() or path.read_text() != body:
                sys.exit(f'documentation drift: {path.relative_to(ROOT)}')
        else:
            path.write_text(body)
    for body in outputs.values():
        # Every generated relative file link must resolve inside this checkout.
        for target in re.findall(r'\]\(([^)]+)\)', body):
            destination = (USER / target).resolve()
            if not destination.is_relative_to(ROOT) or not destination.is_file():
                sys.exit(f'broken generated link: {target}')
    print(f'docs reference OK: {pages} guides, {controllers} controllers, {schemas} schema sources; all generated links resolve')


if __name__ == '__main__':
    main()
