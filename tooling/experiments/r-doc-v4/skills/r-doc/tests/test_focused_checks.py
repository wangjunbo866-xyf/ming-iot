from pathlib import Path
import json
import subprocess
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).parents[1] / 'scripts'))
import check_docs
from helpers import write_file


class FocusedCheckTests(unittest.TestCase):
    def test_named_files_only_but_outgoing_links_are_checked(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            write_file(root, 'docs/changed.md', '# Changed\n[Target](target.md)\n')
            write_file(root, 'docs/target.md', '# Target\n')
            write_file(root, 'docs/unrelated.md', '[Broken](missing.md)\n')
            self.assertEqual(check_docs.check(root, [Path('docs/changed.md')]), [])
            (root / 'docs/target.md').unlink()
            self.assertEqual([f.code for f in check_docs.check(root, [Path('docs/changed.md')])], ['broken-link'])

    def test_invalid_empty_and_outside_scope_are_errors(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for paths, code in [([], 'scope-empty'), ([Path('missing.md')], 'scope-path'), ([Path('../outside.md')], 'scope-path')]:
                self.assertIn(code, {f.code for f in check_docs.check(root, paths)})
            write_file(root, '.r-doc.yaml', 'exclude: [docs]\n')
            write_file(root, 'docs/changed.md', '# Changed\n')
            self.assertIn('scope-excluded', {f.code for f in check_docs.check(root, [Path('docs/changed.md')])})

    def test_symlink_outside_root_is_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory) / 'project'
            root.mkdir()
            write_file(Path(directory), 'outside.md', '# Outside\n')
            (root / 'linked.md').symlink_to(Path(directory) / 'outside.md')
            self.assertIn('scope-path', {f.code for f in check_docs.check(root, [Path('linked.md')])})

    def test_sensitive_findings_do_not_expose_value(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            sample = 'AKIA' + '1234567890ABCDEF'
            write_file(root, 'changed.md', sample)
            findings = check_docs.check(root, [Path('changed.md')])
            self.assertIn('sensitive-content', {f.code for f in findings})
            self.assertNotIn(sample, str(findings))

    def test_cli_reports_limited_coverage_and_failure_exit(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            write_file(root, 'changed.md', '[Missing](missing.md)\n')
            result = subprocess.run([sys.executable, check_docs.__file__, '--root', str(root), '--files', 'changed.md'], text=True, capture_output=True)
            self.assertEqual(result.returncode, 1)
            report = json.loads(result.stdout)
            self.assertEqual(report['scope'], 'explicit_files')
            self.assertIn('metadata_and_lifecycle', report['not_checked'])
            self.assertIn('stage_gates', report['not_checked'])


if __name__ == '__main__':
    unittest.main()
