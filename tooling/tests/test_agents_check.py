import contextlib
import importlib.util
import io
import json
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location('agents_check', Path(__file__).resolve().parents[1] / 'agents_check.py')
agents_check = importlib.util.module_from_spec(spec)
spec.loader.exec_module(agents_check)


class AgentsCheckTests(unittest.TestCase):
    def test_boundaries_newlines_exit_codes_and_no_mutation(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            path = root / 'AGENTS.md'
            for count, result, code in [(114, 'PASS', 0), (115, 'WARN', 0), (120, 'WARN', 0), (121, 'ERROR', 1)]:
                for newline in ['\n', '\r\n']:
                    for terminal in [False, True]:
                        with self.subTest(count=count, newline=newline, terminal=terminal):
                            # Blank lines count too. Unicode separators are not physical newlines.
                            content = newline.join(['entry\u2028text'] + [''] * (count - 2) + ['last'])
                            content += newline if terminal else ''
                            before = content.encode('utf-8')
                            path.write_bytes(before)
                            output = io.StringIO()
                            with contextlib.redirect_stdout(output):
                                actual_code = agents_check.main([], root=root)
                            report = json.loads(output.getvalue())
                            self.assertEqual((report['lines'], report['result'], actual_code), (count, result, code))
                            self.assertEqual(path.read_bytes(), before)

    def test_unreadable_empty_invalid_and_oversized_inputs_fail(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            path = root / 'AGENTS.md'
            self.assertEqual(agents_check.check(root)['result'], 'ERROR')
            for content in [b'', b' \n', b'\xff', b'x' * (agents_check.MAX_BYTES + 1)]:
                path.write_bytes(content)
                self.assertEqual(agents_check.check(root)['result'], 'ERROR')
            path.unlink()
            path.mkdir()
            self.assertEqual(agents_check.check(root)['result'], 'ERROR')

    def test_symlink_is_not_read_and_utf8_bom_is_supported(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            target = root / 'other.md'
            target.write_text('private text\n')
            path = root / 'AGENTS.md'
            path.symlink_to(target)
            self.assertEqual(agents_check.check(root)['result'], 'ERROR')
            path.unlink()
            path.write_bytes(b'\xef\xbb\xbfentry\r\n')
            self.assertEqual(agents_check.check(root)['lines'], 1)


if __name__ == '__main__':
    unittest.main()
