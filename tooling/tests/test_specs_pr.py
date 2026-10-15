"""Exercise actual Git comparisons and PR event input in isolated repositories."""
import json
from pathlib import Path
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).resolve().parents[1] / 'specs_pr.py'


class SpecsPRTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name) / 'repo'
        self.root.mkdir()
        self.git('init', '-q')
        self.git('config', 'user.name', 'Test')
        self.git('config', 'user.email', 'test@example.invalid')
        self.mapping = {'schema_version': 1, 'modules': []}
        for mid in ('a', 'b'):
            folder = f'docs/specs/{mid}'
            self.mapping['modules'].append(dict(id=mid, paths=[f'backend/{mid}/**', folder+'/**'],
                                                spec=folder+'/spec.md', entry=folder+'/README.md'))
            self.write(folder+'/spec.md', '---\nversion: v1\nstatus: effective\n---\n<a id="rule"></a>\nCurrent rule\n')
            self.write(folder+'/README.md', '[Current](spec.md)\n<a id="patches"></a>\n[Patch](patches/P1/change.md)\n')
            self.write(folder+'/patches/P1/change.md', '---\nstatus: confirmed\nrevision: R1\n---\n[Rule](../../spec.md#rule)\n[Back](../../README.md#patches)\n')
            self.write_json(folder+'/patches/P1/check.json', dict(schema_version=2, module=mid, task='task-'+mid,
                type='patch', claim=folder+'/patches/P1/change.md', confirmation_source='fixture task',
                spec=folder+'/spec.md#rule', spec_version='v1', backlink=folder+'/README.md#patches',
                write_scope=[f'backend/{mid}/**', folder+'/**']))
            self.write(f'backend/{mid}/main.go', 'package fixture\n')
        self.save_map()
        self.write('README.md', '# Fixture\n')
        self.write('docs/old.md', '# Old\n[Historical broken link](missing.md)\n')
        self.base = self.commit()

    def git(self, *args):
        return subprocess.check_output(['git', *args], cwd=self.root, text=True, stderr=subprocess.PIPE).strip()

    def write(self, name, value):
        p = self.root / name
        p.parent.mkdir(parents=True, exist_ok=True)
        p.write_text(value, encoding='utf-8')

    def write_json(self, name, value):
        self.write(name, json.dumps(value))

    def edit(self, name, old, new):
        self.write(name, (self.root/name).read_text().replace(old, new))

    def save_map(self):
        self.write_json('tooling/specs-map.json', self.mapping)

    def commit(self):
        self.git('add', '.')
        self.git('commit', '-qm', 'fixture')
        return self.git('rev-parse', 'HEAD')

    def claim(self, mid='a'):
        return dict(module=mid, kind='no_behavior_change', spec_version='v1', clauses=['rule'],
                    paths=[f'backend/{mid}/**', f'docs/specs/{mid}/**'], reason='Restore the current rule')

    def decl(self, *claims, exceptions=None, archives=None):
        return dict(schema_version=1, modules=list(claims), exceptions=exceptions or [], archives=archives or [])

    def run_check(self, declaration, base=None, head=None, event=False, body=None, event_head=None):
        head = head or self.git('rev-parse', 'HEAD')
        file = Path(self.tmp.name)/'input.json'
        if event:
            body = body if body is not None else 'PR explanation\n```ming-specs\n'+json.dumps(declaration)+'\n```\n'
            file.write_text(json.dumps({'pull_request': {'base': {'sha': base or self.base},
                'head': {'sha': event_head or head}, 'body': body}}))
            args = ['--event', str(file)]
        else:
            file.write_text(json.dumps(declaration))
            args = ['--base', base or self.base, '--head', head, '--declaration', str(file)]
        run = subprocess.run(['python3', str(SCRIPT), *args], cwd=self.root, capture_output=True, text=True)
        self.assertTrue(run.stdout, run.stderr)
        return run.returncode, json.loads(run.stdout)

    def good(self, declaration, **kw):
        code, result = self.run_check(declaration, **kw)
        self.assertEqual(code, 0, result)
        self.assertEqual(result['result'], 'STRUCTURE_OK_REVIEW_REQUIRED')
        self.assertEqual(result['semantic_review'], 'NOT_RUN')
        return result

    def bad(self, declaration, fragment, **kw):
        code, result = self.run_check(declaration, **kw)
        self.assertEqual(code, 1, result)
        self.assertIn(fragment, result['reason'])

    def change_code(self, mid='a'):
        self.write(f'backend/{mid}/main.go', 'package fixture\n// corrected\n')

    def test_multiple_modules_use_union_of_claims(self):
        self.change_code('a'); self.change_code('b'); self.commit()
        report = self.good(self.decl(self.claim('a'), self.claim('b')))
        self.assertEqual(len(report['changed_files']), 2)
        self.bad(self.decl(self.claim('a')), 'backend/b/main.go')

    def test_mixed_behavior_patch_and_lightweight(self):
        self.change_code('a'); self.change_code('b')
        self.edit('docs/specs/a/spec.md', 'Current rule', 'Changed rule')
        self.commit()
        claim = dict(module='a', kind='patch', change='docs/specs/a/patches/P1/check.json', revision='R1')
        self.good(self.decl(claim, self.claim('b')))
        claim['revision'] = 'R0'
        self.bad(self.decl(claim, self.claim('b')), 'stale Patch revision')

    def test_behavior_patch_requires_main_spec_consolidation(self):
        self.change_code(); self.commit()
        self.bad(self.decl(dict(module='a', kind='patch', change='docs/specs/a/patches/P1/check.json', revision='R1')),
                 'must consolidate')

    def test_patch_scope_cannot_cover_another_module(self):
        self.change_code(); self.change_code('b')
        self.edit('docs/specs/a/spec.md', 'Current rule', 'Changed rule'); self.commit()
        self.bad(self.decl(dict(module='a', kind='patch', change='docs/specs/a/patches/P1/check.json', revision='R1')),
                 'backend/b/main.go')

    def test_missing_backlink_blocks_even_for_light_claim(self):
        self.edit('docs/specs/a/README.md', '[Patch](patches/P1/change.md)', 'Missing backlink'); self.commit()
        self.bad(self.decl(self.claim()), '反向指针')

    def test_missing_current_link_and_anchor_block(self):
        self.edit('docs/specs/a/README.md', '[Current](spec.md)', '[Current](missing.md)'); self.commit()
        self.bad(self.decl(self.claim()), 'does not link current spec')

    def test_light_claim_requires_current_version_and_clause(self):
        self.change_code(); self.commit()
        claim = self.claim(); claim['spec_version'] = 'v0'
        self.bad(self.decl(claim), 'stale spec version')
        claim = self.claim(); claim['clauses'] = ['nonexistent']
        self.bad(self.decl(claim), 'stable clause')

    def test_light_document_edit_needs_no_new_patch(self):
        self.edit('docs/specs/a/README.md', '[Current]', '[Current specification]'); self.commit()
        self.good(self.decl(self.claim()))
        self.assertEqual(len(list(self.root.glob('docs/specs/a/patches/*'))), 1)

    def test_unmapped_code_never_disappears(self):
        self.write('backend/other/new.go', 'package other\n'); self.commit()
        self.bad(self.decl(), 'backend/other/new.go')
        report = self.good(self.decl(exceptions=[dict(kind='unmapped', paths=['backend/other/**'],
                         reason='No current module yet; review scope explicitly', reference='README.md')]))
        self.assertEqual(report['unmapped_requires_review'], ['backend/other/new.go'])

    def test_doc_exception_cannot_hide_registered_change(self):
        self.edit('docs/specs/a/spec.md', 'Current rule', 'Changed rule'); self.commit()
        self.bad(self.decl(exceptions=[dict(kind='documentation', paths=['docs/**'], reason='typo')]), 'cannot bypass')

    def test_unmapped_exception_requires_readable_reference(self):
        self.write('backend/other/new.go', 'package other'); self.commit()
        self.bad(self.decl(exceptions=[dict(kind='unmapped', paths=['backend/other/**'], reason='gap',
                                           reference='missing.md')]), 'missing regular file')

    def test_archive_preserves_bytes_without_revalidating_historical_links(self):
        dst = 'docs/wiki/legacy/old/原 资料.md'
        self.write(dst, (self.root/'docs/old.md').read_text()); (self.root/'docs/old.md').unlink(); self.commit()
        report = self.good(self.decl(archives=[{'from': 'docs/old.md', 'to': dst, 'reason': 'Historical reference'}]))
        self.assertEqual(set(report['changed_files']), {'docs/old.md', dst})
        self.edit(dst, '# Old', '# Edited'); self.commit()
        self.bad(self.decl(archives=[{'from':'docs/old.md', 'to':dst, 'reason':'archive'}]), 'preserve source blob')

    def test_archive_rejects_mode_change(self):
        dst = 'docs/wiki/legacy/old.md'
        self.write(dst, (self.root/'docs/old.md').read_text()); (self.root/'docs/old.md').unlink()
        (self.root/dst).chmod(0o755); self.commit()
        self.bad(self.decl(archives=[{'from':'docs/old.md', 'to':dst, 'reason':'archive'}]), 'preserve source blob')

    def test_history_addition_is_explicit_and_cannot_hide_deleted_source(self):
        self.write('docs/wiki/legacy/new.md', 'Old record\n'); self.commit()
        self.good(self.decl(exceptions=[dict(kind='history', paths=['docs/wiki/legacy/new.md'], reason='Imported history')]))
        (self.root/'docs/old.md').unlink(); self.commit()
        self.bad(self.decl(exceptions=[dict(kind='history', paths=['docs/wiki/legacy/**'], reason='history')]), 'docs/old.md')

    def test_rename_checks_both_paths(self):
        self.git('mv', 'backend/a/main.go', 'backend/a/renamed.go'); self.commit()
        claim = self.claim(); claim['paths'] = ['backend/a/renamed.go']
        self.bad(self.decl(claim), 'backend/a/main.go')
        self.good(self.decl(self.claim()))

    def test_shared_path_requires_each_module(self):
        for m in self.mapping['modules']:
            m['paths'].append('backend/shared.go')
        self.save_map(); self.write('backend/shared.go', 'package shared'); self.base = self.commit()
        self.write('backend/shared.go', 'package shared\n// fix'); self.commit()
        claims = [self.claim('a'), self.claim('b')]
        for claim in claims: claim['paths'].append('backend/shared.go')
        self.bad(self.decl(claims[0]), 'backend/shared.go')
        self.good(self.decl(*claims))

    def test_removing_mapping_cannot_bypass_module(self):
        self.mapping['modules'].pop(); self.save_map(); self.change_code('b'); self.commit()
        self.bad(self.decl(), 'cannot silently disappear')

    def test_narrowing_mapping_keeps_old_ownership(self):
        self.mapping['modules'][0]['paths'] = ['docs/specs/a/**']; self.save_map()
        self.change_code(); self.commit()
        exc = [dict(kind='tooling', paths=['tooling/specs-map.json'], reason='mapping update')]
        self.bad(self.decl(exceptions=exc), 'backend/a/main.go')
        self.good(self.decl(self.claim(), exceptions=exc))

    def test_target_branch_advance_uses_three_dot(self):
        self.change_code(); head = self.commit()
        self.git('checkout', '-qb', 'target', self.base)
        self.write('backend/other/target.go', 'Only target changed'); target = self.commit()
        self.git('checkout', '-q', '--detach', head)
        report = self.good(self.decl(self.claim()), base=target, head=head)
        self.assertEqual(report['requested_base'], target)
        self.assertEqual(report['merge_base'], self.base)
        self.assertEqual(report['changed_files'], ['backend/a/main.go'])

    def test_unrelated_histories_block(self):
        self.change_code(); head = self.commit()
        self.git('checkout', '--orphan', 'unrelated')
        self.git('commit', '-qm', 'unrelated root')
        self.bad(self.decl(self.claim()), 'merge-base', base=self.git('rev-parse', 'HEAD'), head=head)

    def test_dirty_worktree_does_not_repair_candidate(self):
        self.edit('docs/specs/a/README.md', '[Patch](patches/P1/change.md)', 'Missing backlink'); head = self.commit()
        self.edit('docs/specs/a/README.md', 'Missing backlink', '[Patch](patches/P1/change.md)')
        self.bad(self.decl(self.claim()), '反向指针', head=head)

    def test_dirty_worktree_not_claimed_as_checked(self):
        self.change_code(); head = self.commit()
        self.write('backend/a/main.go', 'dirty')
        report = self.good(self.decl(self.claim()), head=head)
        self.assertTrue(report['worktree_changes_not_checked'])

    def test_event_binds_actual_head_and_body(self):
        self.change_code(); self.commit()
        report = self.good(self.decl(self.claim()), event=True)
        self.assertEqual(report['input_kind'], 'event_file')
        self.bad(self.decl(self.claim()), 'checkout of PR head', event=True, event_head=self.base)

    def test_event_missing_duplicate_or_malformed_declarations_block(self):
        self.change_code(); self.commit()
        for body in ('Nothing', '```ming-specs\n{}\n```\n```ming-specs\n{}\n```\n',
                     '```ming-specs\n{"schema_version":1,"schema_version":1}\n```\n'):
            with self.subTest(body=body):
                code, report = self.run_check(self.decl(), event=True, body=body)
                self.assertEqual(code, 1, report)

    def test_broad_wildcard_and_unknown_kind_block(self):
        self.change_code(); self.commit()
        claim = self.claim(); claim['paths'] = ['**']
        self.bad(self.decl(claim), 'literal paths')
        claim = self.claim(); claim['kind'] = 'lightweight'
        self.bad(self.decl(claim), 'unsupported claim kind')

    def test_absorbed_unchanged_history_does_not_pin_current_version(self):
        folder = 'docs/specs/a'
        self.edit(folder+'/patches/P1/change.md', 'confirmed', 'absorbed'); self.base = self.commit()
        self.edit(folder+'/spec.md', 'version: v1', 'version: v2'); self.commit()
        claim = self.claim(); claim['spec_version'] = 'v2'
        report = self.good(self.decl(claim))
        self.assertIn(folder+'/patches/P1/check.json', report['historical_projections_not_revalidated'])

    def test_old_absorbed_patch_cannot_authorize_new_code(self):
        self.edit('docs/specs/a/patches/P1/change.md', 'confirmed', 'absorbed'); self.base = self.commit()
        self.change_code(); self.edit('docs/specs/a/spec.md', 'Current rule', 'Changed rule'); self.commit()
        self.bad(self.decl(dict(module='a', kind='patch', change='docs/specs/a/patches/P1/check.json', revision='R1')),
                 'historical Patch cannot authorize')


if __name__ == '__main__':
    unittest.main()
