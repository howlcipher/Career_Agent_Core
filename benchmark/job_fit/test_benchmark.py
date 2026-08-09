import json
import tempfile
import unittest
from pathlib import Path

from benchmark import (
    MODEL_INPUT_MODES,
    load_cohort,
    rank_agreement,
    verify_safe_directory,
)


class BenchmarkSafetyTest(unittest.TestCase):
    def test_model_input_modes_match_intended_architectures(self):
        self.assertEqual(MODEL_INPUT_MODES["nomic_embed_text"], "title")
        self.assertEqual(MODEL_INPUT_MODES["upply_bge_small_jobs"], "full")
        self.assertEqual(MODEL_INPUT_MODES["techwolf_jobbert_v2"], "title")

    def test_rank_agreement_uses_only_common_rows(self):
        result = rank_agreement(
            {"job-001": 1.0, "job-002": 2.0, "left-only": 3.0},
            {"job-001": 10.0, "job-002": 20.0, "right-only": 0.0},
        )
        self.assertEqual(result["common_count"], 2)
        self.assertAlmostEqual(result["spearman"], 1.0)

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
