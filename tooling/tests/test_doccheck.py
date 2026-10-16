"""Regression contracts for bounded reads, navigation and source identity."""
import copy
import hashlib
import json
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import doccheck
from doclinks import markdown_anchors, navigation_targets, validation_targets
import knowledge
import yaml


class DocumentTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.config = {
            'schema_version': 2,
            'entrypoints': {'home': 'README.md', 'source_registry': 'docs/index.yaml'},
            'maintained_documents': ['README.md', 'docs/current.md'],
            'discovered_skills': [], 'excluded_reference_directories': ['third_party/airmq'],
            'document_scope': {'schema_version': 1, 'discovery_roots': ['docs'],
                               'navigation_roots': ['README.md'],
                               'historical': ['docs/history/**'], 'reference': ['docs/vendor/**']}}
        self.item = {'source_id': 'current', 'path': 'docs/current.md', 'revision': '1.0',
                     'status': 'draft', 'lifecycle': 'current',
                     'body_bindings': {'revision': 'frontmatter.version', 'status': 'frontmatter.status'}}
        self.materials = [self.item]
        self.put('README.md', '[current](docs/current.md#规则)\n')
        self.put('docs/current.md', '---\nversion: "1.0"\nstatus: draft\n---\n# 规则\n')

    def put(self, name, body):
        path = self.root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(body, encoding='utf-8')

    def run_check(self):
        self.put('tooling/knowledge.json', json.dumps(self.config))
        self.put('docs/index.yaml', yaml.safe_dump({'schema_version': 1, 'materials': self.materials}))
        return knowledge.check(self.root)

    def codes(self, result):
        return {f['code'] for f in result['findings']}

    def test_valid_contract_and_no_file_mutations(self):
        self.assertEqual(self.run_check()['result'], 'PASS')
        before = {str(p): hashlib.sha256(p.read_bytes()).hexdigest() for p in self.root.rglob('*') if p.is_file()}
        result = knowledge.check(self.root)
        after = {str(p): hashlib.sha256(p.read_bytes()).hexdigest() for p in self.root.rglob('*') if p.is_file()}
        self.assertEqual(before, after)
        self.assertEqual(result['bodies_read'], ['README.md', 'docs/current.md'])
        self.assertEqual(result['external_urls'], 'NOT_FETCHED')
        self.assertEqual(result['semantic_review'], 'NOT_RUN')

    def test_broken_file_anchor_and_reference(self):
        for body, code in [('[x](missing.md)', 'invalid-link'),
                           ('[x](docs/current.md#missing)', 'missing-anchor'),
                           ('[x][undefined]', 'undefined-reference'),
                           ('![](missing.png)', 'invalid-link')]:
            with self.subTest(body=body):
                self.put('README.md', body)
                self.assertIn(code, self.codes(self.run_check()))

    def test_supported_paths_references_and_anchors(self):
        self.put('docs/规则 (v2).md', '# 中文标题\n\n小节\n---\n')
        self.config['maintained_documents'].append('docs/规则 (v2).md')
        self.put('README.md', '[current](docs/current.md#%E8%A7%84%E5%88%99)\n'
                 '[中文](<docs/规则 (v2).md#中文标题> "caption")\n'
                 '[ref][r]\n[r]: <docs/规则 (v2).md#小节> "title"\n')
        self.assertEqual(self.run_check()['problems'], [])
        self.put('docs/x(v2).md', '# X\n')
        self.config['maintained_documents'].append('docs/x(v2).md')
        with (self.root/'README.md').open('a') as f:
            f.write('[parentheses](docs/x(v2).md#x)')
        self.assertEqual(self.run_check()['result'], 'PASS')

    def test_images_and_unused_definitions_do_not_confer_reachability(self):
        for body in ['![current](docs/current.md)', '[unused]: docs/current.md\n']:
            with self.subTest(body=body):
                self.put('README.md', body)
                self.assertIn('unreachable-document', self.codes(self.run_check()))

    def test_reference_shortcut_at_end_is_navigation(self):
        self.put('README.md', '[current]: docs/current.md\n\n[current]')
        self.assertEqual(self.run_check()['result'], 'PASS')

    def test_duplicate_reference_uses_first_definition(self):
        self.put('README.md', '[current][id]\n\n[id]: missing.md\n[id]: docs/current.md\n')
        result = self.run_check()
        self.assertIn('invalid-link', self.codes(result))
        self.assertIn('unreachable-document', self.codes(result))

    def test_blank_paragraph_cannot_create_navigation(self):
        self.put('README.md', '[current](docs/current.md\n\n)\n')
        self.assertIn('unreachable-document', self.codes(self.run_check()))

    def test_parentheses_in_titles_do_not_change_destination(self):
        for title in ['"hello ("', "'hello )'", '(ordinary title)', '"hello )"']:
            with self.subTest(title=title):
                self.put('README.md', f'[current](docs/current.md {title})\n')
                self.assertEqual(self.run_check()['result'], 'PASS')

    def test_isolated_cycle_remains_unreachable(self):
        self.put('README.md', '# root')
        self.put('docs/other.md', '[current](current.md)')
        self.config['maintained_documents'].append('docs/other.md')
        with (self.root/'docs/current.md').open('a') as f:
            f.write('[other](other.md)')
        result = self.run_check()
        self.assertEqual(sum(f['code'] == 'unreachable-document' for f in result['findings']), 2)

    def test_new_docs_must_be_classified_but_history_is_not_recursively_read(self):
        self.put('docs/new.md', '[bad](not-here.md)')
        self.assertIn('unclassified-document', self.codes(self.run_check()))
        (self.root/'docs/new.md').unlink()
        path = self.root/'docs/history/old.md'; path.parent.mkdir(); path.write_bytes(b'\xff')
        result = self.run_check()
        self.assertEqual(result['result'], 'PASS')
        self.assertNotIn('docs/history/old.md', result['bodies_read'])

    def test_historical_anchor_never_reads_excluded_body(self):
        self.put('docs/history/old.md', '[broken](never.md)')
        with (self.root/'README.md').open('a') as f:
            f.write('[old](docs/history/old.md#ancient)')
        original = doccheck.read_text
        def guarded(root, path):
            self.assertNotEqual(path, 'docs/history/old.md', 'anchor check crossed read scope')
            return original(root, path)
        with patch.object(doccheck, 'read_text', side_effect=guarded):
            result = self.run_check()
        self.assertEqual(result['result'], 'PASS')
        self.assertEqual(result['unverified_fragments'][0]['fragment'], 'ancient')

    def test_paths_cannot_escape_enter_private_or_follow_symlinks(self):
        self.put('.local/private.md', 'private')
        (self.root/'docs/link.md').symlink_to(self.root/'.local/private.md')
        for target in ['../outside', '.local/private.md', 'docs/link.md', '/etc/passwd', '%2e%2e/outside']:
            with self.subTest(target=target):
                self.put('README.md', f'[x]({target}#secret)')
                result = self.run_check()
                self.assertEqual(result['result'], 'FAIL')
                self.assertNotIn('.local/private.md', result['bodies_read'])
        with self.assertRaises(ValueError):
            doccheck.safe_path(self.root, 'docs/link.md')

    def test_document_size_bound(self):
        self.put('docs/current.md', 'a' * (doccheck.MAX_TEXT_BYTES + 1))
        self.assertIn('document-read', self.codes(self.run_check()))

    def test_scope_conflicts_and_unbounded_discovery_rejected(self):
        self.config['document_scope']['discovery_roots'] = ['.']
        self.assertIn('document-config', self.codes(self.run_check()))
        self.config['document_scope']['discovery_roots'] = ['docs']
        self.config['document_scope']['reference'] = ['docs/history/**']
        self.put('docs/history/old.md', 'old')
        self.assertIn('document-config', self.codes(self.run_check()))

    def test_duplicate_id_current_path_and_invalid_related(self):
        self.materials.append(copy.deepcopy(self.item))
        self.assertIn('duplicate-source-id', self.codes(self.run_check()))
        self.materials[-1]['source_id'] = 'other'
        self.assertIn('duplicate-current-source', self.codes(self.run_check()))
        self.materials.pop()
        self.item['related'] = ['missing.py']
        self.assertIn('index-record', self.codes(self.run_check()))

    def test_multiple_historical_records_can_share_a_summary(self):
        self.put('docs/history/summary.md', 'unchanged old summary')
        self.materials.extend({'source_id': f'old-{i}', 'path': 'docs/history/summary.md',
                               'status': 'snapshot', 'revision': f'm{i}-1', 'lifecycle': 'historical'} for i in (1, 2))
        self.assertEqual(self.run_check()['result'], 'PASS')
        self.materials[-1]['body_bindings'] = {'revision': 'frontmatter.version'}
        self.assertIn('index-record', self.codes(self.run_check()))

    def test_version_status_and_malformed_version_fail_structurally(self):
        for metadata in ['version: "2.0"\nstatus: draft', 'version: "1.0"\nstatus: final',
                         'version: []\nstatus: draft']:
            with self.subTest(metadata=metadata):
                self.put('docs/current.md', f'---\n{metadata}\n---\n# 规则\n')
                self.assertIn('index-body-mismatch', self.codes(self.run_check()))

    def test_yaml_duplicate_keys_aliases_and_wrong_shape_rejected(self):
        for body in ['schema_version: 1\nschema_version: 1\nmaterials: []',
                     'x: &x [a]\ny: *x', '- not-a-mapping', 'x: [unterminated']:
            with self.subTest(body=body), self.assertRaises(ValueError):
                doccheck.yaml_mapping(body)

    def relation(self):
        self.put('docs/history/old.md', '[current](../current.md)\n[unrelated old broken link](gone.md)')
        with (self.root/'docs/current.md').open('a') as f:
            f.write('[old](history/old.md)')
        self.materials.append({'source_id': 'old', 'path': 'docs/history/old.md',
                               'status': 'snapshot', 'revision': 'old-1', 'lifecycle': 'historical'})
        self.item['supersedes'] = [{'source_id': 'old', 'scope': 'next priorities only; retain evidence'}]

    def test_scoped_supersession_reads_only_explicit_endpoints(self):
        self.relation()
        self.put('docs/history/unrelated.md', '[bad](gone.md)')
        result = self.run_check()
        self.assertEqual(result['result'], 'PASS')
        self.assertEqual(result['supersession_bodies_read'], ['docs/current.md', 'docs/history/old.md'])
        self.assertNotIn('docs/history/unrelated.md', result['bodies_read'])

    def test_supersession_requires_backlinks_target_and_no_self_or_cycle(self):
        self.relation()
        self.put('docs/history/old.md', 'no backlink')
        self.assertIn('missing-supersession-link', self.codes(self.run_check()))
        self.put('docs/history/old.md', '[current](../current.md)')
        for sid in ['missing', 'current']:
            self.item['supersedes'][0]['source_id'] = sid
            self.assertIn('invalid-supersession', self.codes(self.run_check()))
        self.item['supersedes'][0]['source_id'] = 'old'
        self.materials[-1]['supersedes'] = [{'source_id': 'current', 'scope': 'loop'}]
        self.assertIn('supersession-cycle', self.codes(self.run_check()))

    def test_supersession_scope_and_distinct_bodies_required(self):
        self.relation()
        self.item['supersedes'][0]['scope'] = ''
        self.assertIn('index-record', self.codes(self.run_check()))
        self.item['supersedes'][0]['scope'] = 'priority'
        self.materials[-1]['path'] = self.item['path']
        self.assertIn('supersession-read', self.codes(self.run_check()))


class MarkdownParsingTests(unittest.TestCase):
    def test_code_comments_and_escaped_syntax_are_not_links(self):
        text = ('```md\n[bad](absent.md)\n```\n~~~\n[x](missing)\n~~~\n'
                '`[bad](absent)` <!-- [bad](absent) -->\n'
                '\\[escaped](absent)\n``[x](a) ` [y](b)``\n[real](ok.md)')
        self.assertEqual([x.raw_target for x in validation_targets(text)], ['ok.md'])

    def test_heading_anchors_unicode_duplicate_collisions_and_explicit_id(self):
        text = '# 标题\n# 标题\n# 标题-1\n\n小节\n---\n<a id="stable"></a>\n'
        self.assertEqual(markdown_anchors(text), {'标题', '标题-1', '标题-1-1', '小节', 'stable'})
        self.assertNotIn('hidden', markdown_anchors('```md\n# hidden\n```'))

    def test_images_excluded_and_used_reference_definitions_not_double_counted(self):
        body = '[real][id]\n![img](img.png)\n[id]: real.md\n[unused]: other.md\n'
        self.assertEqual([t.raw_target for t in navigation_targets(body)], ['real.md'])
        self.assertEqual([t.raw_target for t in validation_targets(body)], ['real.md', 'img.png', 'other.md'])


if __name__ == '__main__':
    unittest.main()
