from pathlib import Path
import sys
import tempfile
import unittest
import yaml

sys.path.insert(0, str(Path(__file__).parents[1] / 'scripts'))
import audit_docs
from rdoc.config import load_project_config
from helpers import write_file


PROFILE = {
    'type_field': 'document_kind',
    'required_fields': ['document_kind', 'status', 'delivery_issue', 'code_repository'],
    'statuses_by_type': {'spec': ['draft', 'approved', 'superseded'], 'review': ['pass', 'changes_requested']},
}


class ProjectProfileTests(unittest.TestCase):
    def project(self, root, profile=PROFILE):
        config = {'governance_level': 'minimal', 'required_document_types': ['spec']}
        if profile is not None:
            config['metadata'] = profile
        write_file(root, '.r-doc.yaml', yaml.safe_dump(config))
        write_file(root, 'AGENTS.md', '[Spec](docs/spec.md)\n[Review](docs/review.md)\n')
        for kind, status in [('spec', 'approved'), ('review', 'pass')]:
            write_file(root, f'docs/{kind}.md', f'---\ndocument_kind: {kind}\nstatus: {status}\ndelivery_issue: ming-iot#I123\ncode_repository: git:example.invalid/team/product\n---\n# Document\n')

    def test_project_metadata_is_accepted_without_rewriting(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.project(root)
            before = {p: p.read_bytes() for p in root.rglob('*') if p.is_file()}
            self.assertEqual(audit_docs.audit(root), [])
            self.assertTrue(all(p.read_bytes() == data for p, data in before.items()))

    def test_defaults_still_reject_incompatible_metadata(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.project(root, None)
            codes = {f.code for f in audit_docs.audit(root)}
            self.assertTrue({'metadata-field', 'metadata-status', 'required-document-type'}.issubset(codes))

    def test_kind_specific_status_and_required_fields_fail_closed(self):
        for replacement, expected in [('status: pass', 'metadata-status'), ('document_kind: unknown', 'metadata-type'), ('delivery_issue:', 'metadata-field')]:
            with self.subTest(replacement=replacement), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                self.project(root)
                path = root / 'docs/spec.md'
                field = replacement.split(':')[0]
                lines = [replacement if line.startswith(field + ':') else line for line in path.read_text().splitlines()]
                path.write_text('\n'.join(lines))
                self.assertIn(expected, {f.code for f in audit_docs.audit(root)})

    def test_invalid_profiles_do_not_disable_validation(self):
        invalid = [None, {}, {'type_field': 'document_kind'}, {**PROFILE, 'extra': True},
                   {**PROFILE, 'required_fields': []}, {**PROFILE, 'required_fields': ['document_kind']},
                   {**PROFILE, 'statuses_by_type': {}}, {**PROFILE, 'statuses_by_type': {'spec': []}},
                   {**PROFILE, 'type_field': ['document_kind']}]
        for profile in invalid:
            with self.subTest(profile=profile), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                write_file(root, '.r-doc.yaml', yaml.safe_dump({'metadata': profile}))
                config, problems = load_project_config(root)
                self.assertIsNone(config.metadata)
                self.assertIn('config-metadata', {p.code for p in problems})
                self.assertIn('config-metadata', {f.code for f in audit_docs.audit(root)})

    def test_duplicate_ids_remain_detected(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.project(root)
            for path in (root / 'docs').glob('*.md'):
                path.write_text(path.read_text().replace('---\n', '---\nid: same\n', 1))
            self.assertIn('duplicate-id', {f.code for f in audit_docs.audit(root)})

    def test_directory_profiles_remain_distinct(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.project(root)
            config = yaml.safe_load((root / '.r-doc.yaml').read_text())
            config['metadata_overrides'] = {'docs/map': {**PROFILE, 'statuses_by_type': {'spec': ['current']}}}
            write_file(root, '.r-doc.yaml', yaml.safe_dump(config))
            current = (root / 'docs/spec.md').read_text().replace('status: approved', 'status: current')
            write_file(root, 'docs/map/spec.md', current)
            write_file(root, 'docs/map-other/spec.md', current)
            write_file(root, 'AGENTS.md', '[Spec](docs/spec.md)\n[Review](docs/review.md)\n[Map](docs/map/spec.md)\n[Other](docs/map-other/spec.md)\n')
            findings = audit_docs.audit(root)
            self.assertEqual([(f.path, f.code) for f in findings], [('docs/map-other/spec.md', 'metadata-status')])
            write_file(root, 'docs/map/spec.md', current.replace('status: current', 'status: approved'))
            self.assertIn('docs/map/spec.md', {f.path for f in audit_docs.audit(root) if f.code == 'metadata-status'})

    def test_longest_path_profile_wins(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            profile = {**PROFILE, 'statuses_by_type': {'spec': ['current']}}
            write_file(root, '.r-doc.yaml', yaml.safe_dump({'metadata': PROFILE, 'metadata_overrides': {'docs': PROFILE, 'docs/map': profile}}))
            config, errors = load_project_config(root)
            self.assertEqual(errors, [])
            self.assertEqual(config.metadata_for(root, root/'docs/map/topic.md').statuses_by_type['spec'], ('current',))

    def test_invalid_override_paths_fail_closed(self):
        for prefix in ['', '.', '../docs', '/docs', 'C:/docs', 'docs/../other', 'docs//map', 'docs\\map']:
            with self.subTest(prefix=prefix), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                write_file(root, '.r-doc.yaml', yaml.safe_dump({'metadata_overrides': {prefix: PROFILE}}))
                _, problems = load_project_config(root)
                self.assertIn('config-metadata-overrides', {p.code for p in problems})


if __name__ == '__main__':
    unittest.main()
