"""Generic concurrent Ollama executor.

This service owns no prompts and no model choice. Callers (the Go
agent) supply the system/user prompt text for every call, the model,
and every Ollama option. The service's only job is to fan a batch of
chat calls out to Ollama concurrently and report back per-call
results/errors without letting one failure take down the others.
"""

import concurrent.futures

import requests
from fastapi import FastAPI
from pydantic import BaseModel, Field, field_validator

app = FastAPI(title="Career Agent NLP Microservice")

DEFAULT_HOST = "http://localhost:11434"
DEFAULT_KEEP_ALIVE = "30m"
DEFAULT_TEMPERATURE = -1
DEFAULT_NUM_CTX = 0
DEFAULT_TIMEOUT_SECONDS = 7200


class OllamaCall(BaseModel):
    key: str = Field(min_length=1)
    system: str = ""
    prompt: str = ""


class GenerateRequest(BaseModel):
    host: str = DEFAULT_HOST
    model: str = Field(min_length=1)
    keep_alive: str = DEFAULT_KEEP_ALIVE
    num_ctx: int = DEFAULT_NUM_CTX
    timeout_seconds: int = DEFAULT_TIMEOUT_SECONDS
    temperature: float = DEFAULT_TEMPERATURE
    calls: list[OllamaCall] = Field(min_length=1)

    @field_validator("calls")
    @classmethod
    def keys_must_be_unique(
        cls, calls: list[OllamaCall]
    ) -> list[OllamaCall]:
        seen = set()
        dupes = set()
        for call in calls:
            if call.key in seen:
                dupes.add(call.key)
            seen.add(call.key)
        if dupes:
            raise ValueError(
                "duplicate call keys: " + ", ".join(sorted(dupes))
            )
        return calls


class GenerateResponse(BaseModel):
    results: dict[str, str]
    errors: dict[str, str]


def call_ollama(
    call: OllamaCall,
    host: str,
    model: str,
    keep_alive: str,
    num_ctx: int,
    timeout_seconds: int,
    temperature: float,
) -> str:
    """Run a single /api/chat call and return the stripped text.

    Raises RuntimeError on any failure; the caller is responsible for
    turning that into a per-key error entry rather than aborting the
    whole batch.
    """
    messages = []
    if call.system:
        messages.append({"role": "system", "content": call.system})
    messages.append({"role": "user", "content": call.prompt})

    payload = {
        "model": model,
        "messages": messages,
        "stream": False,
        "keep_alive": keep_alive,
    }

    options = {}
    if temperature >= 0:
        options["temperature"] = temperature
    if num_ctx > 0:
        options["num_ctx"] = num_ctx
    if options:
        payload["options"] = options

    try:
        resp = requests.post(
            f"{host}/api/chat", json=payload, timeout=timeout_seconds
        )
    except Exception as e:
        raise RuntimeError(f"failed to reach ollama at {host}: {e}")

    if resp.status_code != 200:
        raise RuntimeError(
            f"ollama returned HTTP {resp.status_code}: {resp.text}"
        )

    try:
        data = resp.json()
    except Exception as e:
        raise RuntimeError(f"failed to parse ollama response: {e}")

    error = data.get("error") or ""
    if error:
        raise RuntimeError(f"ollama error: {error}")

    content = data.get("message", {}).get("content", "")
    return content.strip()


@app.get("/health")
def health():
    return {"status": "ok"}


@app.post("/generate", response_model=GenerateResponse)
def generate(req: GenerateRequest):
    results: dict[str, str] = {}
    errors: dict[str, str] = {}

    with concurrent.futures.ThreadPoolExecutor(
        max_workers=len(req.calls)
    ) as executor:
        futures = {
            executor.submit(
                call_ollama,
                call,
                req.host,
                req.model,
                req.keep_alive,
                req.num_ctx,
                req.timeout_seconds,
                req.temperature,
            ): call.key
            for call in req.calls
        }
        for future, key in futures.items():
            try:
                results[key] = future.result()
            except Exception as e:
                errors[key] = str(e)

    return GenerateResponse(results=results, errors=errors)
