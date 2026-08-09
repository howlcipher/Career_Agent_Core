# Job-fit embedding benchmark

This directory is an isolated research harness. It is not imported by Career Agent's production scorer, ranking path, or Assisted Apply queue. Private production-derived inputs and all downloaded weights belong under ignored `benchmark_results/` paths.

The benchmark compares three pinned models:

- the installed `nomic-embed-text` production embedding baseline;
- `upply-org/bge-small-jobs-data-embedding` at revision `042d48864ea832df6d22abaca1870c6b8d59a07a`;
- `TechWolf/JobBERT-v2` at revision `a480476925abdf9d97621e56aa38abbb572fe343`.

The downloader fetches only JSON, tokenizer text, ONNX, and safetensors artifacts from pinned revisions, then verifies Git blob hashes or SHA-256 hashes. The runner rejects other file types and always loads JobBERT with `trust_remote_code=False` and `local_files_only=True`.

Inputs follow each architecture rather than forcing incomparable models through
one artificial text shape. The existing `nomic-embed-text` baseline receives
the job title, which is the closest privacy-safe reproduction of production's
company-plus-title `fit_similarity` input. Upply receives the sanitized title
and posting text, truncated to its documented 64-token maximum. JobBERT-v2
receives titles because its training objective is job-title normalization.

## Local workflow

Generate a private cohort through the read-only Go extractor:

```bash
go run ./cmd/benchmark-job-fit -mode extract
```

Download and verify the pinned model files explicitly:

```bash
bash scripts/download_job_fit_models.sh
```

Create an isolated Python environment, install `requirements-lock.txt` with the PyTorch CPU wheel index, and run:

```bash
python -m pip install \
  --extra-index-url https://download.pytorch.org/whl/cpu \
  --requirement benchmark/job_fit/requirements-lock.txt

python benchmark/job_fit/benchmark.py run \
  --output benchmark_results/private/benchmark_results.json
```

The first run may have zero human ranking metrics. Review `benchmark_results/private/human_review.md`, enter labels in the companion CSV, then rerun. Empty labels are never inferred or replaced with model-derived pseudo-ground-truth.

Run deterministic metric tests without third-party dependencies:

```bash
python -m unittest discover -s benchmark/job_fit -p 'test_*.py'
python -m flake8 --config benchmark/job_fit/.flake8 benchmark/job_fit
```
