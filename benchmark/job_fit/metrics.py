"""Dependency-free ranking and discrimination metrics for the benchmark."""

from __future__ import annotations

import math
import statistics
from collections.abc import Sequence


def _average_ranks(values: Sequence[float]) -> list[float]:
    indexed = sorted(enumerate(values), key=lambda pair: (pair[1], pair[0]))
    ranks = [0.0] * len(values)
    start = 0
    while start < len(indexed):
        end = start + 1
        while end < len(indexed) and indexed[end][1] == indexed[start][1]:
            end += 1
        average_rank = (start + end - 1) / 2.0
        for position in range(start, end):
            ranks[indexed[position][0]] = average_rank
        start = end
    return ranks


def _pearson(left: Sequence[float], right: Sequence[float]) -> float:
    if len(left) != len(right) or not left:
        return math.nan
    left_mean = statistics.fmean(left)
    right_mean = statistics.fmean(right)
    numerator = sum(
        (left_value - left_mean) * (right_value - right_mean)
        for left_value, right_value in zip(left, right, strict=True)
    )
    left_scale = math.sqrt(sum((value - left_mean) ** 2 for value in left))
    right_scale = math.sqrt(sum((value - right_mean) ** 2 for value in right))
    if left_scale == 0 or right_scale == 0:
        return math.nan
    return numerator / (left_scale * right_scale)


def spearman_correlation(labels: Sequence[float], scores: Sequence[float]) -> float:
    """Return Spearman's rho with average ranks for ties."""
    if len(labels) != len(scores) or len(labels) < 2:
        return math.nan
    return _pearson(_average_ranks(labels), _average_ranks(scores))


def pairwise_accuracy(labels: Sequence[float], scores: Sequence[float]) -> float:
    """Return correct preference pairs; a tied model score receives half credit."""
    if len(labels) != len(scores):
        return math.nan
    correct = 0.0
    compared = 0
    for left in range(len(labels)):
        for right in range(left + 1, len(labels)):
            label_delta = labels[left] - labels[right]
            if label_delta == 0:
                continue
            compared += 1
            score_delta = scores[left] - scores[right]
            if score_delta == 0:
                correct += 0.5
            elif (label_delta > 0) == (score_delta > 0):
                correct += 1.0
    return correct / compared if compared else math.nan


def _discounted_gain(labels: Sequence[float]) -> float:
    return sum(
        ((2.0**label) - 1.0) / math.log2(position + 2.0)
        for position, label in enumerate(labels)
    )


def ndcg_at_k(labels: Sequence[float], scores: Sequence[float], k: int) -> float:
    if len(labels) != len(scores) or not labels or k <= 0:
        return math.nan
    limit = min(k, len(labels))
    order = sorted(range(len(scores)), key=lambda index: (-scores[index], index))[:limit]
    ranked_labels = [labels[index] for index in order]
    ideal_labels = sorted(labels, reverse=True)[:limit]
    ideal_gain = _discounted_gain(ideal_labels)
    return _discounted_gain(ranked_labels) / ideal_gain if ideal_gain else math.nan


def precision_at_k(
    labels: Sequence[float],
    scores: Sequence[float],
    k: int,
    relevant_label: float = 2.0,
) -> float:
    if len(labels) != len(scores) or not labels or k <= 0:
        return math.nan
    limit = min(k, len(labels))
    order = sorted(range(len(scores)), key=lambda index: (-scores[index], index))[:limit]
    relevant = sum(labels[index] >= relevant_label for index in order)
    return relevant / limit


def rank_percentiles(values: Sequence[float]) -> list[float]:
    """Map values to tie-aware [0, 1] ranks before hybrid sensitivity tests."""
    if not values:
        return []
    ranks = _average_ranks(values)
    if len(values) == 1:
        return [0.5]
    return [rank / (len(values) - 1) for rank in ranks]


def _percentile(sorted_values: Sequence[float], quantile: float) -> float:
    position = quantile * (len(sorted_values) - 1)
    lower = math.floor(position)
    upper = math.ceil(position)
    if lower == upper:
        return sorted_values[lower]
    weight = position - lower
    return sorted_values[lower] * (1.0 - weight) + sorted_values[upper] * weight


def discrimination_metrics(values: Sequence[float]) -> dict[str, float | int]:
    """Describe usable spread at six-decimal precision, not cosmetic decimals."""
    if not values:
        return {
            "count": 0,
            "distinct_values_6dp": 0,
            "standard_deviation": math.nan,
            "p10": math.nan,
            "p90": math.nan,
            "percentile_spread_p90_p10": math.nan,
            "top_decile_separation": math.nan,
            "ties_in_top_20_6dp": 0,
        }
    rounded = [round(float(value), 6) for value in values]
    sorted_values = sorted(float(value) for value in values)
    p10 = _percentile(sorted_values, 0.10)
    p90 = _percentile(sorted_values, 0.90)
    top_count = max(1, math.ceil(len(sorted_values) * 0.10))
    top_mean = statistics.fmean(sorted_values[-top_count:])
    remainder = sorted_values[:-top_count]
    remainder_mean = statistics.fmean(remainder) if remainder else top_mean
    top_values = sorted(rounded, reverse=True)[: min(20, len(rounded))]
    ties = len(top_values) - len(set(top_values))
    return {
        "count": len(values),
        "distinct_values_6dp": len(set(rounded)),
        "standard_deviation": statistics.pstdev(values),
        "p10": p10,
        "p90": p90,
        "percentile_spread_p90_p10": p90 - p10,
        "top_decile_separation": top_mean - remainder_mean,
        "ties_in_top_20_6dp": ties,
    }


def ranking_metrics(labels: Sequence[float], scores: Sequence[float]) -> dict[str, float]:
    return {
        "spearman": spearman_correlation(labels, scores),
        "pairwise_accuracy": pairwise_accuracy(labels, scores),
        "ndcg_at_10": ndcg_at_k(labels, scores, 10),
        "ndcg_at_20": ndcg_at_k(labels, scores, 20),
        "precision_at_10": precision_at_k(labels, scores, 10),
        "precision_at_20": precision_at_k(labels, scores, 20),
    }
