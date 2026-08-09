#!/usr/bin/env python3
"""Run isolated local job-fit embedding benchmarks without production writes."""

from __future__ import annotations

import argparse
import csv
import hashlib
import json
import math
import os
import resource
import subprocess
import sys
import time
import urllib.request
from dataclasses import dataclass
from pathlib import Path
from typing import Any

from metrics import discrimination_metrics, rank_percentiles, ranking_metrics


MODEL_REVISIONS = {
    "nomic_embed_text": "0a109f422b47e3a30ba2b10eca18548e944e8a23073ee3f3e947efcf3c45e59f",
    "upply_bge_small_jobs": "042d48864ea832df6d22abaca1870c6b8d59a07a",
    "techwolf_jobbert_v2": "a480476925abdf9d97621e56aa38abbb572fe343",
}
SAFE_MODEL_SUFFIXES = {".json", ".txt", ".onnx", ".safetensors"}


@dataclass(frozen=True)
class Profile:
    roles: list[str]
    skills: list[str]
    experience_years: str

    @property
    def embedding_text(self) -> str:
        return (
            "Target roles: "
            + ", ".join(self.roles)
            + ". Skills and experience: "
            + ", ".join(self.skills)
            + f". Years of experience: {self.experience_years}."
        )


def load_json(path: Path) -> dict[str, Any]:
    with path.open(encoding="utf-8") as source:
        value = json.load(source)
    if not isinstance(value, dict):
        raise ValueError(f"{path} must contain a JSON object")
    return value


def load_cohort(path: Path) -> list[dict[str, Any]]:
    payload = load_json(path)
    jobs = payload.get("jobs")
    if not isinstance(jobs, list) or not jobs:
        raise ValueError("cohort must contain at least one job")
    required = {"benchmark_id", "title", "description"}
    for job in jobs:
        if not isinstance(job, dict) or not required.issubset(job):
            raise ValueError("cohort job is missing required fields")
        forbidden = {"url", "company", "company_name", "database_id"}
        if forbidden.intersection(job):
            raise ValueError("cohort contains a forbidden source identifier")
    return jobs


def load_profile(path: Path) -> Profile:
    try:
        import yaml
    except ImportError as error:
        raise RuntimeError("PyYAML is unavailable; install requirements-lock.txt") from error
    with path.open(encoding="utf-8") as source:
        payload = yaml.safe_load(source)
    if not isinstance(payload, dict):
        raise ValueError("profile must contain a YAML mapping")
    roles = [str(value).strip() for value in payload.get("roles", []) if str(value).strip()]
    skills = [str(value).strip() for value in payload.get("skills", []) if str(value).strip()]
    if not roles or not skills:
        raise ValueError("profile must contain non-empty roles and skills")
    return Profile(
        roles=roles,
        skills=skills,
        experience_years=str(payload.get("experience_years", "unspecified")),
    )


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for block in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def verify_safe_directory(path: Path) -> None:
    if not path.is_dir():
        raise FileNotFoundError(f"model directory is unavailable: {path}")
    unsafe = [
        candidate
        for candidate in path.rglob("*")
        if candidate.is_file() and candidate.suffix.lower() not in SAFE_MODEL_SUFFIXES
    ]
    if unsafe:
        relative = ", ".join(str(candidate.relative_to(path)) for candidate in unsafe)
        raise RuntimeError(f"model directory contains disallowed file types: {relative}")


def verify_weight(path: Path, expected_sha256: str) -> None:
    if not path.is_file():
        raise FileNotFoundError(f"required model weight is unavailable: {path}")
    actual = sha256(path)
    if actual != expected_sha256:
        raise RuntimeError(f"weight hash mismatch for {path.name}")


def directory_size(path: Path) -> int:
    return sum(candidate.stat().st_size for candidate in path.rglob("*") if candidate.is_file())


def normalize_rows(values: Any) -> Any:
    import numpy as np

    array = np.asarray(values, dtype=np.float32)
    norms = np.linalg.norm(array, axis=1, keepdims=True)
    return array / np.maximum(norms, 1e-12)


def cosine_scores(job_vectors: Any, profile_vector: Any) -> list[float]:
    import numpy as np

    jobs = normalize_rows(job_vectors)
    profile = normalize_rows(np.asarray(profile_vector).reshape(1, -1))[0]
    return [float(value) for value in jobs @ profile]


class OllamaEmbeddingModel:
    key = "nomic_embed_text"
    size_bytes = 274_302_450
    external_service = True

    def __init__(self, endpoint: str, text_mode: str):
        self.endpoint = endpoint.rstrip("/") + "/api/embed"
        self.text_mode = text_mode

    def _embed_batch(self, texts: list[str]) -> list[list[float]]:
        body = json.dumps(
            {
                "model": "nomic-embed-text:latest",
                "input": texts,
                "truncate": True,
                "keep_alive": "10m",
            }
        ).encode("utf-8")
        request = urllib.request.Request(
            self.endpoint,
            data=body,
            headers={"Content-Type": "application/json"},
            method="POST",
        )
        with urllib.request.urlopen(request, timeout=180) as response:
            payload = json.load(response)
        embeddings = payload.get("embeddings")
        if not isinstance(embeddings, list) or len(embeddings) != len(texts):
            raise RuntimeError("Ollama returned an unexpected embedding response")
        return embeddings

    def _embed(self, texts: list[str]) -> list[list[float]]:
        embeddings = []
        for start in range(0, len(texts), 10):
            embeddings.extend(self._embed_batch(texts[start : start + 10]))
        return embeddings

    def cold_probe(self) -> None:
        self._embed(["job-fit benchmark cold-load probe"])

    def prepare_profile(self, profile: Profile) -> list[float]:
        return self._embed([profile.embedding_text])[0]

    def score(self, jobs: list[dict[str, Any]], prepared: list[float]) -> list[float]:
        texts = (
            [job["title"] for job in jobs]
            if self.text_mode == "title"
            else [job_text(job) for job in jobs]
        )
        return cosine_scores(self._embed(texts), prepared)


class UpplyONNXModel:
    key = "upply_bge_small_jobs"
    external_service = False

    def __init__(self, path: Path):
        verify_safe_directory(path)
        verify_weight(
            path / "model_quantized.onnx",
            "3f1ca2ff91c049e0b860232d38080f80851942b70ab09b875a2cb64427c844ef",
        )
        try:
            import onnxruntime
            from tokenizers import Tokenizer
        except ImportError as error:
            raise RuntimeError("ONNX benchmark dependencies are unavailable") from error
        self.path = path
        self.size_bytes = directory_size(path)
        self.tokenizer = Tokenizer.from_file(str(path / "tokenizer.json"))
        self.tokenizer.enable_truncation(max_length=64)
        self.tokenizer.enable_padding(pad_id=0, pad_token="[PAD]")
        self.session = onnxruntime.InferenceSession(
            str(path / "model_quantized.onnx"),
            providers=["CPUExecutionProvider"],
        )
        self.input_names = {item.name for item in self.session.get_inputs()}

    def cold_probe(self) -> None:
        self._embed(["job-fit benchmark cold-load probe"])

    def _embed(self, texts: list[str]) -> Any:
        import numpy as np

        encoded = self.tokenizer.encode_batch(texts)
        arrays = {
            "input_ids": np.asarray([item.ids for item in encoded], dtype=np.int64),
            "attention_mask": np.asarray(
                [item.attention_mask for item in encoded], dtype=np.int64
            ),
            "token_type_ids": np.asarray(
                [item.type_ids for item in encoded], dtype=np.int64
            ),
        }
        feeds = {name: value for name, value in arrays.items() if name in self.input_names}
        output = self.session.run(None, feeds)[0]
        if output.ndim == 3:
            output = output[:, 0, :]
        return normalize_rows(output)

    def prepare_profile(self, profile: Profile) -> Any:
        return self._embed([profile.embedding_text])[0]

    def score(self, jobs: list[dict[str, Any]], prepared: Any) -> list[float]:
        return cosine_scores(self._embed([job_text(job) for job in jobs]), prepared)


class JobBERTModel:
    key = "techwolf_jobbert_v2"
    external_service = False

    def __init__(self, path: Path):
        verify_safe_directory(path)
        weights = {
            "model.safetensors": "955fda98dcb7765d37617ace3e0a13c8695ac6d4c2e27d4b85c0e9454222117a",
            "2_Asym/140216480444672_Dense/model.safetensors": (
                "32b0721292561cc684d5e71dc13b2d5fc9e86405cb085194500fbb0232530e45"
            ),
            "2_Asym/140216480445248_Dense/model.safetensors": (
                "647eaba5e180e77e55e20b2b1f22cb83a2686bbb0881f6c326627d8d9b5f603d"
            ),
        }
        for relative, expected in weights.items():
            verify_weight(path / relative, expected)
        with (path / "modules.json").open(encoding="utf-8") as source:
            modules_payload = json.load(source)
        allowed = {
            "sentence_transformers.models.Transformer",
            "sentence_transformers.models.Pooling",
            "sentence_transformers.models.Asym",
        }
        module_types = {item.get("type") for item in modules_payload}
        if not module_types.issubset(allowed):
            raise RuntimeError("JobBERT declares a non-standard Sentence Transformers module")
        asym = load_json(path / "2_Asym/config.json")
        dense_types = set(asym.get("types", {}).values())
        if dense_types != {"sentence_transformers.models.Dense"}:
            raise RuntimeError("JobBERT declares a non-standard asymmetric module")
        try:
            import torch
            from sentence_transformers import SentenceTransformer
        except ImportError as error:
            raise RuntimeError(
                "Sentence Transformers benchmark dependencies are unavailable"
            ) from error
        self.torch = torch
        self.path = path
        self.size_bytes = directory_size(path)
        self.model = SentenceTransformer(
            str(path),
            device="cpu",
            local_files_only=True,
            trust_remote_code=False,
        )

    def cold_probe(self) -> None:
        self._encode(["job-fit benchmark cold-load probe"], "anchor")

    def _encode(self, texts: list[str], branch: str) -> Any:
        import numpy as np
        from sentence_transformers.util import batch_to_device

        chunks = []
        for start in range(0, len(texts), 16):
            features = self.model.tokenize(texts[start : start + 16])
            features = batch_to_device(features, self.model.device)
            features["text_keys"] = [branch]
            with self.torch.no_grad():
                output = self.model.forward(features)["sentence_embedding"]
            chunks.append(output.cpu().numpy())
        return normalize_rows(np.concatenate(chunks))

    def prepare_profile(self, profile: Profile) -> dict[str, Any]:
        return {
            "roles": self._encode(profile.roles, "anchor"),
            "skills": self._encode(profile.skills, "positive"),
        }

    def score(self, jobs: list[dict[str, Any]], prepared: dict[str, Any]) -> list[float]:
        import numpy as np

        job_vectors = self._encode([job["title"] for job in jobs], "anchor")
        concepts = np.concatenate([prepared["roles"], prepared["skills"]])
        similarities = job_vectors @ concepts.T
        top_count = min(3, similarities.shape[1])
        top = np.partition(similarities, -top_count, axis=1)[:, -top_count:]
        return [float(value) for value in np.mean(top, axis=1)]


def job_text(job: dict[str, Any]) -> str:
    return f"Job title: {job['title']}\nResponsibilities and requirements: {job['description']}"


def build_model(
    key: str, model_root: Path, ollama_endpoint: str, text_mode: str
) -> Any:
    if key == "nomic_embed_text":
        return OllamaEmbeddingModel(ollama_endpoint, text_mode)
    if key == "upply_bge_small_jobs":
        return UpplyONNXModel(model_root / "upply-org/bge-small-jobs-data-embedding")
    if key == "techwolf_jobbert_v2":
        return JobBERTModel(model_root / "TechWolf/JobBERT-v2")
    raise ValueError(f"unknown model key: {key}")


def peak_rss_mib() -> float:
    return resource.getrusage(resource.RUSAGE_SELF).ru_maxrss / 1024.0


def run_worker(arguments: argparse.Namespace) -> dict[str, Any]:
    jobs = load_cohort(arguments.cohort)
    profile = load_profile(arguments.profile)

    load_start = time.perf_counter()
    model = build_model(
        arguments.model,
        arguments.model_root,
        arguments.ollama_endpoint,
        arguments.text_mode,
    )
    model.cold_probe()
    cold_load_seconds = time.perf_counter() - load_start

    profile_start = time.perf_counter()
    prepared = model.prepare_profile(profile)
    profile_prepare_seconds = time.perf_counter() - profile_start
    model.score(jobs[:1], prepared)

    latency_jobs = jobs[: min(20, len(jobs))]
    warm_start = time.perf_counter()
    warm_cpu_start = time.process_time()
    for job in latency_jobs:
        model.score([job], prepared)
    warm_wall = time.perf_counter() - warm_start
    warm_cpu = time.process_time() - warm_cpu_start

    batch_jobs = [jobs[index % len(jobs)] for index in range(100)]
    batch_start = time.perf_counter()
    batch_cpu_start = time.process_time()
    batch_scores = model.score(batch_jobs, prepared)
    batch_wall = time.perf_counter() - batch_start
    batch_cpu = time.process_time() - batch_cpu_start

    if len(jobs) == 100 and all(
        batch_jobs[index]["benchmark_id"] == jobs[index]["benchmark_id"]
        for index in range(100)
    ):
        scores = batch_scores
    else:
        scores = model.score(jobs, prepared)
    logical_cpus = max(1, os.cpu_count() or 1)
    result = {
        "model_key": arguments.model,
        "text_mode": arguments.text_mode,
        "revision": MODEL_REVISIONS[arguments.model],
        "scores": [
            {"benchmark_id": job["benchmark_id"], "similarity": score}
            for job, score in zip(jobs, scores, strict=True)
        ],
        "runtime": {
            "model_size_bytes": model.size_bytes,
            "cold_load_seconds": cold_load_seconds,
            "profile_prepare_seconds": profile_prepare_seconds,
            "warm_latency_seconds_per_job": warm_wall / len(latency_jobs),
            "warm_cpu_utilization_percent": (
                None
                if model.external_service
                else 100.0 * warm_cpu / max(warm_wall * logical_cpus, 1e-12)
            ),
            "batch_100_seconds": batch_wall,
            "batch_100_cpu_utilization_percent": (
                None
                if model.external_service
                else 100.0 * batch_cpu / max(batch_wall * logical_cpus, 1e-12)
            ),
            "cpu_utilization_note": (
                "external Ollama runner; inspect the server process separately"
                if model.external_service
                else "process CPU time divided by wall time and logical CPU count"
            ),
            "peak_process_rss_mib": peak_rss_mib(),
            "logical_cpu_count": logical_cpus,
        },
    }
    return result


def read_labels(path: Path) -> dict[str, int]:
    if not path.exists():
        return {}
    labels: dict[str, int] = {}
    with path.open(newline="", encoding="utf-8") as source:
        for row in csv.DictReader(source):
            value = (row.get("human_label_0_3") or "").strip()
            if not value:
                continue
            label = int(value)
            if label < 0 or label > 3:
                raise ValueError(f"human label outside 0-3 for {row.get('benchmark_id')}")
            labels[row["benchmark_id"]] = label
    return labels


def finite_or_none(value: Any) -> Any:
    if isinstance(value, dict):
        return {key: finite_or_none(item) for key, item in value.items()}
    if isinstance(value, list):
        return [finite_or_none(item) for item in value]
    if isinstance(value, float) and not math.isfinite(value):
        return None
    return value


def signal_analysis(
    signal: dict[str, float], labels: dict[str, int]
) -> dict[str, Any]:
    ordered_ids = sorted(signal)
    values = [signal[benchmark_id] for benchmark_id in ordered_ids]
    result: dict[str, Any] = {"discrimination": discrimination_metrics(values)}
    labeled_ids = [benchmark_id for benchmark_id in ordered_ids if benchmark_id in labels]
    result["human_labeled_count"] = len(labeled_ids)
    result["ranking"] = (
        ranking_metrics(
            [labels[benchmark_id] for benchmark_id in labeled_ids],
            [signal[benchmark_id] for benchmark_id in labeled_ids],
        )
        if labeled_ids
        else None
    )
    return finite_or_none(result)


def run_parent(arguments: argparse.Namespace) -> dict[str, Any]:
    jobs = load_cohort(arguments.cohort)
    labels = read_labels(arguments.labels)
    signals: dict[str, dict[str, float]] = {}
    for key, cohort_field in (
        ("current_fit_score", "model_fit_score"),
        ("current_embedding_similarity", "model_embedding_similarity"),
    ):
        signals[key] = {
            job["benchmark_id"]: float(job[cohort_field])
            for job in jobs
            if job.get(cohort_field) is not None
        }

    workers = []
    runtime = {}
    for model_key in MODEL_REVISIONS:
        worker_path = arguments.output.parent / f"{model_key}_worker.json"
        command = [
            sys.executable,
            str(Path(__file__).resolve()),
            "worker",
            "--model",
            model_key,
            "--cohort",
            str(arguments.cohort),
            "--profile",
            str(arguments.profile),
            "--model-root",
            str(arguments.model_root),
            "--ollama-endpoint",
            arguments.ollama_endpoint,
            "--text-mode",
            "full",
            "--output",
            str(worker_path),
        ]
        subprocess.run(command, check=True)
        worker = load_json(worker_path)
        workers.append(worker_path)
        signals[model_key] = {
            item["benchmark_id"]: float(item["similarity"])
            for item in worker["scores"]
        }
        runtime[model_key] = worker["runtime"]

    analysis = {key: signal_analysis(value, labels) for key, value in signals.items()}
    hybrids: dict[str, Any] = {}
    if labels:
        for candidate in ("upply_bge_small_jobs", "techwolf_jobbert_v2"):
            common = sorted(
                set(signals["current_fit_score"])
                & set(signals[candidate])
                & set(labels)
            )
            if not common:
                continue
            current_ranks = rank_percentiles(
                [signals["current_fit_score"][item] for item in common]
            )
            candidate_ranks = rank_percentiles([signals[candidate][item] for item in common])
            for embedding_weight in (0.25, 0.50, 0.75):
                hybrid_scores = [
                    embedding_weight * embedding
                    + (1.0 - embedding_weight) * current
                    for embedding, current in zip(candidate_ranks, current_ranks, strict=True)
                ]
                name = f"{candidate}_embedding_{int(embedding_weight * 100)}"
                hybrids[name] = finite_or_none(
                    ranking_metrics([labels[item] for item in common], hybrid_scores)
                )

    result = finite_or_none(
        {
            "schema_version": 1,
            "generated_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
            "cohort_size": len(jobs),
            "human_label_count": len(labels),
            "human_labels_are_ground_truth": bool(labels),
            "signals": analysis,
            "hybrid_sensitivity": hybrids if labels else None,
            "runtime": runtime,
            "limitations": (
                "No winner-quality ranking metrics are available until the user supplies "
                "explicit labels."
                if not labels
                else None
            ),
        }
    )
    for path in workers:
        path.unlink(missing_ok=True)
    return result


def write_private_json(path: Path, payload: dict[str, Any]) -> None:
    path.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    temporary = path.with_suffix(path.suffix + ".partial")
    with temporary.open("w", encoding="utf-8") as destination:
        json.dump(payload, destination, indent=2, allow_nan=False)
        destination.write("\n")
    temporary.chmod(0o600)
    temporary.replace(path)


def parse_arguments() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    subparsers = parser.add_subparsers(dest="command", required=True)
    for command in ("run", "worker"):
        child = subparsers.add_parser(command)
        child.add_argument(
            "--cohort",
            type=Path,
            default=Path("benchmark_results/private/job_fit_cohort.json"),
        )
        child.add_argument("--profile", type=Path, default=Path("profile.yaml"))
        child.add_argument(
            "--model-root", type=Path, default=Path("benchmark_results/models")
        )
        child.add_argument(
            "--ollama-endpoint", default="http://127.0.0.1:11434"
        )
        child.add_argument("--text-mode", choices=("full", "title"), default="full")
        child.add_argument("--output", type=Path, required=True)
    run_parser = subparsers.choices["run"]
    run_parser.add_argument(
        "--labels",
        type=Path,
        default=Path("benchmark_results/private/human_labels.csv"),
    )
    worker_parser = subparsers.choices["worker"]
    worker_parser.add_argument("--model", choices=tuple(MODEL_REVISIONS), required=True)
    return parser.parse_args()


def main() -> int:
    arguments = parse_arguments()
    try:
        payload = run_worker(arguments) if arguments.command == "worker" else run_parent(arguments)
        write_private_json(arguments.output, payload)
    except Exception as error:  # fail cleanly at the CLI boundary
        print(f"job-fit benchmark failed: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
