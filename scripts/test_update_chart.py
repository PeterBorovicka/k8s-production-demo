import tempfile
import unittest
from pathlib import Path

from scripts.update_chart import update_chart, update_values


class UpdateChartTest(unittest.TestCase):
    def test_updates_images_and_versions(self):
        with tempfile.TemporaryDirectory() as directory:
            values = Path(directory) / "values.yaml"
            chart = Path(directory) / "Chart.yaml"
            values.write_text(
                "images:\n"
                "  frontend:\n"
                "    repository: old/frontend\n"
                '    tag: "old"\n'
                "    pullPolicy: IfNotPresent\n"
                "  backend:\n"
                "    repository: old/backend\n"
                '    tag: "old"\n',
                encoding="utf-8",
            )
            chart.write_text('version: 1.2.3\nappVersion: "old"\n', encoding="utf-8")

            update_values(values, "new/frontend", "new/backend", "abc123")
            new_version = update_chart(chart, "abc123")

            self.assertEqual(new_version, "1.2.4")
            self.assertIn("repository: new/frontend", values.read_text(encoding="utf-8"))
            self.assertIn('tag: "abc123"', values.read_text(encoding="utf-8"))
            self.assertEqual(chart.read_text(encoding="utf-8"), 'version: 1.2.4\nappVersion: "abc123"\n')

    def test_fails_when_required_field_is_missing(self):
        with tempfile.TemporaryDirectory() as directory:
            values = Path(directory) / "values.yaml"
            values.write_text("images:\n  frontend:\n    repository: old\n", encoding="utf-8")
            with self.assertRaises(ValueError):
                update_values(values, "front", "back", "tag")


if __name__ == "__main__":
    unittest.main()

