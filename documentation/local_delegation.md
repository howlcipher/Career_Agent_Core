# Safe local-model delegation

`cmd/localdelegate` is a framework-independent, local-Ollama-only harness for bounded repository work. It is intentionally not an autonomous coding agent.

## Boundary

The command can make one request to the configured local Ollama `/api/chat` endpoint. It cannot invoke a shell, apply a patch, control a browser, send email, access the application database, read credentials, commit or push Git changes, or make architecture, security, or concurrency decisions.

It also checks the production agent's single-instance lock before contacting Ollama and refuses while the agent is running. Unlike a benchmark, there is no force override: local delegation is background work and must yield to applications.

The brief is bounded to 32 KiB and rejected if it contains obvious credential markers. Do not include `.env`, `pii.yaml`, application data, generated documents, browser content, raw logs containing personal data, or credentials in a brief.

## Two phases

1. `propose` asks the model only for a strict JSON proposal. It includes the finding, root cause, planned files, implementation summary, success and failure tests, risks, open questions, and `ready_to_edit`. Unknown fields, malformed JSON, sensitive paths, missing tests, and oversized output are rejected.
2. `patch` requires a human reviewer identifier and the SHA-256 digest printed for the exact reviewed proposal. It produces a candidate unified diff artifact only. The diff must touch only the proposal's planned relative paths; credential files, Git metadata, application data, and task journals are forbidden. It is never applied automatically.

The orchestration agent remains responsible for selecting the backlog item, deciding whether a model is suitable, reviewing every artifact, applying any accepted change, running final tests, maintaining journals and backlogs, and committing or pushing.

## Example

Create a sanitized brief manually, then run:

```bash
go run ./cmd/localdelegate \
  -phase propose \
  -model qwen3:4b-instruct \
  -brief-file /tmp/sanitized_brief.md \
  -proposal-file /tmp/proposal.json
```

Review the proposal and its printed digest. Only after review, request a candidate patch:

```bash
go run ./cmd/localdelegate \
  -phase patch \
  -model qwen3:4b-instruct \
  -brief-file /tmp/sanitized_brief.md \
  -proposal-file /tmp/proposal.json \
  -approved-by "reviewer-name" \
  -approved-proposal-sha256 "digest-printed-by-propose" \
  -patch-file /tmp/candidate.patch
```

Inspect the patch independently. Applying it, if appropriate, is a separate deliberate repository action by the reviewer.
