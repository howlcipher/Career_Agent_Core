# nlp_service

A tiny FastAPI wrapper around a local Ollama server that runs a batch
of chat completions concurrently.

This service is **optional**. The Go agent (`cmd/agent`) generates
tailored documents in-process by default and only calls this service
when the `NLP_SERVICE_URL` environment variable is set. You do not
need to run this service to use the agent.

The service owns no prompts and no model choice. Every system
prompt, user prompt, and the model name are supplied by the caller on
each request. This service's only job is to fan a batch of Ollama
`/api/chat` calls out concurrently and report back per-call
results/errors without letting one failing call take down the
others.

## Setup

From inside `nlp_service/`:

```bash
python3 -m venv venv
venv/bin/pip install -r requirements.txt
```

## Running

```bash
cd nlp_service
venv/bin/uvicorn main:app --port 8000
```

## Endpoints

### `GET /health`

Returns HTTP 200 without contacting Ollama. Used to check that the
service process itself is up.

Response:

```json
{"status": "ok"}
```

### `POST /generate`

Runs every entry in `calls` concurrently against Ollama's
`/api/chat` endpoint and returns each result keyed by the caller's
`key`. `model` and a non-empty `calls` list are required; everything
else has a default.

Request:

```json
{
  "host": "http://localhost:11434",
  "model": "qwen3:30b-instruct",
  "keep_alive": "30m",
  "num_ctx": 12000,
  "timeout_seconds": 7200,
  "temperature": -1,
  "calls": [
    {"key": "resume", "system": "system prompt text", "prompt": "user prompt text"},
    {"key": "cover_letter", "system": "...", "prompt": "..."}
  ]
}
```

Response (always HTTP 200 for a well-formed request, even if every
call failed -- a given key appears in exactly one of `results` or
`errors`):

```json
{
  "results": {"resume": "generated text", "cover_letter": "generated text"},
  "errors": {"interview_prep": "ollama returned HTTP 404: model \"llama3\" not found"}
}
```

Notes on field defaults and behavior:

- `temperature` is only sent to Ollama when `>= 0` (negative means
  "use the model default").
- `num_ctx` is only sent when `> 0`.
- A malformed request (missing/empty `model`, empty `calls`, or
  duplicate `key` values across calls) returns a 4xx, never a 500 and
  never a silent success.
