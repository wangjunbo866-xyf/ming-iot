import importlib.util
import json
from pathlib import Path
import tempfile
import unittest

spec=importlib.util.spec_from_file_location('knowledge',Path(__file__).resolve().parents[1]/'knowledge.py')
knowledge=importlib.util.module_from_spec(spec);spec.loader.exec_module(knowledge)

class KnowledgeTests(unittest.TestCase):
    def test_navigation_and_scope_fail_closed(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp);(root/'tooling').mkdir();(root/'docs').mkdir()
            (root/'README.md').write_text('[Docs](docs/index.md)\n')
            (root/'docs/index.md').write_text('current knowledge\n')
            config={'schema_version':2,'entrypoints':{'home':'README.md','source_registry':'docs/index.yaml'},'maintained_documents':['README.md','docs/index.md'],'discovered_skills':[],'excluded_reference_directories':['third_party/airmq']}
            config['document_scope']={'schema_version':1,'discovery_roots':['docs'],'navigation_roots':['README.md'],'historical':[],'reference':[]}
            (root/'docs/index.yaml').write_text('schema_version: 1\nmaterials: []\n')
            (root/'tooling/knowledge.json').write_text(json.dumps(config))
            self.assertEqual(knowledge.check(root)['result'],'PASS')
            (root/'docs/index.md').unlink()
            self.assertEqual(knowledge.check(root)['result'],'FAIL')
            (root/'docs/index.md').write_text('current\n')
            (root/'third_party/airmq').mkdir(parents=True)
            self.assertEqual(knowledge.check(root)['result'],'FAIL')
    def test_paths_cannot_escape_or_follow_external_links(self):
        with tempfile.TemporaryDirectory() as tmp, tempfile.TemporaryDirectory() as other:
            root=Path(tmp);outside=Path(other)/'secret';outside.write_text('outside')
            (root/'link').symlink_to(outside)
            for value in ['../secret',str(outside),'link','missing']:
                with self.subTest(value=value), self.assertRaises(ValueError):knowledge.local_path(root,value)
