from pathlib import Path
import os
import sys
import tempfile
import unittest

sys.path.insert(0, os.environ.get("RDOC_SCRIPTS_ROOT", str(Path(__file__).parents[1] / "scripts")))
import audit_docs
from rdoc.markdown import markdown_targets
from helpers import write_file


class TitledLinkTests(unittest.TestCase):
    def test_angle_destination_with_title_is_a_navigation_edge(self):
        for title in ('"Device status"', "'Device status'", '(Device status)', '"Device ) status"', r'"Device \"status\""'):
            with self.subTest(title=title), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                write_file(root, '.r-doc.yaml', 'governance_level: minimal\n')
                write_file(root, 'AGENTS.md', f'[Device](<docs/device status.md#状态> {title})\n')
                write_file(root, 'docs/device status.md', '# Device\n\n## 状态\n')
                self.assertEqual(audit_docs.audit(root), [])

    def test_missing_titled_target_and_anchor_are_reported(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            write_file(root, '.r-doc.yaml', 'governance_level: minimal\n')
            write_file(root, 'AGENTS.md', '[Missing](<missing.md> "title")\n[Guide](<docs/guide.md#missing> \'title\')\n')
            write_file(root, 'docs/guide.md', '# Guide\n')
            codes = [f.code for f in audit_docs.audit(root)]
            self.assertEqual(sorted(codes), ['broken-anchor', 'broken-link'])

    def test_malformed_angle_titles_are_not_links(self):
        for text in ('[x](<x.md> garbage)', '[x](<x.md>"title")', '[x](<x.md> "unclosed)', '[x](<x.md> (nested (title)))', '[x](<x\n.md> "title")', '[x](<x.md>\n\n"title")', '[x](<x.md> "blank\n\nline")'):
            with self.subTest(text=text):
                self.assertEqual(markdown_targets(text), [])

    def test_whitespace_and_masked_examples(self):
        text = '[real]( <a b.md>\n "title" )\n`[code](<missing.md> "title")`\n<!-- [comment](<missing.md> "title") -->'
        self.assertEqual([t.raw_target for t in markdown_targets(text)], ['<a b.md>'])


if __name__ == '__main__':
    unittest.main()
