import math
import unittest

from metrics import (
    discrimination_metrics,
    ndcg_at_k,
    pairwise_accuracy,
    precision_at_k,
    rank_percentiles,
    ranking_metrics,
    spearman_correlation,
)


class MetricsTest(unittest.TestCase):
    def test_spearman_handles_ties(self):
        self.assertAlmostEqual(
            spearman_correlation([3, 2, 2, 0], [0.9, 0.5, 0.5, 0.1]),
            1.0,
        )

    def test_pairwise_accuracy_ignores_equal_labels_and_halves_score_ties(self):
        self.assertAlmostEqual(pairwise_accuracy([3, 2, 1], [0.9, 0.5, 0.1]), 1.0)
        self.assertAlmostEqual(pairwise_accuracy([3, 2], [0.5, 0.5]), 0.5)
        self.assertTrue(math.isnan(pairwise_accuracy([2, 2], [0.8, 0.1])))

    def test_ndcg_is_one_for_ideal_ranking(self):
        labels = [3, 2, 1, 0]
        scores = [0.9, 0.8, 0.2, 0.1]
        self.assertAlmostEqual(ndcg_at_k(labels, scores, 4), 1.0)

    def test_precision_uses_reasonable_or_strong_as_relevant(self):
        labels = [3, 1, 2, 0]
        scores = [0.9, 0.8, 0.7, 0.1]
        self.assertAlmostEqual(precision_at_k(labels, scores, 3), 2 / 3)

    def test_rank_percentiles_do_not_compare_raw_model_ranges(self):
        self.assertEqual(rank_percentiles([10, 20, 30]), [0.0, 0.5, 1.0])
        self.assertEqual(rank_percentiles([0.1, 0.2, 0.3]), [0.0, 0.5, 1.0])

    def test_discrimination_uses_effective_precision(self):
        result = discrimination_metrics([0.10000001, 0.10000002, 0.2, 0.9])
        self.assertEqual(result["distinct_values_6dp"], 3)
        self.assertEqual(result["ties_in_top_20_6dp"], 1)
        self.assertGreater(result["percentile_spread_p90_p10"], 0)

    def test_ranking_metrics_reports_requested_measures(self):
        labels = [3, 2, 1, 0]
        scores = [0.9, 0.8, 0.2, 0.1]
        result = ranking_metrics(labels, scores)
        self.assertAlmostEqual(result["spearman"], 1.0)
        self.assertAlmostEqual(result["pairwise_accuracy"], 1.0)
        self.assertAlmostEqual(result["ndcg_at_10"], 1.0)
        self.assertAlmostEqual(result["precision_at_10"], 0.5)


if __name__ == "__main__":
    unittest.main()
