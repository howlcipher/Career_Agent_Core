import json
import tempfile
import unittest
from pathlib import Path

from benchmark import load_cohort, verify_safe_directory


class BenchmarkSafetyTest(unittest.TestCase):
    def test_load_cohort_rejects_source_identifiers(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "cohort.json"
            path.write_text(
                json.dumps(
                    {
                        "jobs": [
                            {
                                "benchmark_id": "job-001",
                                "title": "Engineer",
                                "description": "A sufficiently descriptive role.",
                                "url": "https://example.com/private",
                            }
                        ]
                    }
                ),
                encoding="utf-8",
            )
            with self.assertRaisesRegex(ValueError, "forbidden source identifier"):
                load_cohort(path)

    def test_verify_safe_directory_rejects_python_and_pickle(self):
        for suffix in (".py", ".pkl", ".bin"):
            with self.subTest(suffix=suffix), tempfile.TemporaryDirectory() as directory:
                path = Path(directory)
                (path / f"unsafe{suffix}").write_bytes(b"unsafe")
                with self.assertRaisesRegex(RuntimeError, "disallowed file types"):
                    verify_safe_directory(path)

    def test_verify_safe_directory_fails_cleanly_when_model_is_missing(self):
        with tempfile.TemporaryDirectory() as directory:
            missing = Path(directory) / "missing"
            with self.assertRaisesRegex(FileNotFoundError, "model directory is unavailable"):
                verify_safe_directory(missing)


if __name__ == "__main__":
    unittest.main()
