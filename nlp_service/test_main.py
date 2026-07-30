"""Tests for the generic concurrent Ollama executor in main.py.

requests.post is monkeypatched throughout so no real Ollama server is
needed. Each fake records the payload it was called with so tests can
assert on the exact JSON body sent to Ollama.
"""

import threading
import time

import pytest
from fastapi.testclient import TestClient

import main

client = TestClient(main.app)


class FakeResponse:
    def __init__(self, status_code=200, json_data=None, text=""):
        self.status_code = status_code
        self._json_data = json_data if json_data is not None else {}
        self.text = text or str(json_data)

    def json(self):
        return self._json_data


def test_health_does_not_touch_ollama(monkeypatch):
    def boom(*args, **kwargs):
        raise AssertionError("requests.post should not be called")

    monkeypatch.setattr(main.requests, "post", boom)

    resp = client.get("/health")

    assert resp.status_code == 200
    assert resp.json() == {"status": "ok"}


def test_two_calls_return_both_results_and_run_concurrently(monkeypatch):
    calls_in_flight = []
    lock = threading.Lock()
    max_concurrent = [0]

    def fake_post(url, json=None, timeout=None):
        with lock:
            calls_in_flight.append(1)
            max_concurrent[0] = max(max_concurrent[0], len(calls_in_flight))
        # Sleep so both threads are guaranteed to overlap if truly
        # run concurrently.
        time.sleep(0.2)
        with lock:
            calls_in_flight.pop()
        content = f"result for {json['messages'][-1]['content']}"
        return FakeResponse(
            200, {"message": {"content": content}}
        )

    monkeypatch.setattr(main.requests, "post", fake_post)

    body = {
        "model": "qwen3:30b-instruct",
        "calls": [
            {"key": "resume", "system": "sys a", "prompt": "prompt a"},
            {"key": "cover_letter", "system": "sys b", "prompt": "prompt b"},
        ],
    }
    start = time.time()
    resp = client.post("/generate", json=body)
    elapsed = time.time() - start

    assert resp.status_code == 200
    data = resp.json()
    assert data["errors"] == {}
    assert data["results"]["resume"] == "result for prompt a"
    assert data["results"]["cover_letter"] == "result for prompt b"
    # If run sequentially this would take >= 0.4s; concurrently it
    # should be close to 0.2s. Generous margin for slow CI.
    assert elapsed < 0.4
    assert max_concurrent[0] == 2


def test_request_body_shape_sent_to_ollama(monkeypatch):
    captured = {}

    def fake_post(url, json=None, timeout=None):
        captured["url"] = url
        captured["json"] = json
        captured["timeout"] = timeout
        return FakeResponse(200, {"message": {"content": "ok"}})

    monkeypatch.setattr(main.requests, "post", fake_post)

    body = {
        "host": "http://localhost:11434",
        "model": "qwen3:30b-instruct",
        "keep_alive": "45m",
        "num_ctx": 12000,
        "timeout_seconds": 7200,
        "temperature": 0.7,
        "calls": [
            {"key": "resume", "system": "be a resume writer",
             "prompt": "write the resume"},
        ],
    }
    resp = client.post("/generate", json=body)

    assert resp.status_code == 200
    assert captured["url"] == "http://localhost:11434/api/chat"
    sent = captured["json"]
    assert sent["model"] == "qwen3:30b-instruct"
    assert sent["messages"] == [
        {"role": "system", "content": "be a resume writer"},
        {"role": "user", "content": "write the resume"},
    ]
    assert sent["keep_alive"] == "45m"
    assert sent["stream"] is False
    assert sent["options"] == {"temperature": 0.7, "num_ctx": 12000}


def test_num_ctx_zero_and_temperature_negative_are_omitted(monkeypatch):
    captured = {}

    def fake_post(url, json=None, timeout=None):
        captured["json"] = json
        return FakeResponse(200, {"message": {"content": "ok"}})

    monkeypatch.setattr(main.requests, "post", fake_post)

    body = {
        "model": "qwen3:30b-instruct",
        "num_ctx": 0,
        "temperature": -1,
        "calls": [{"key": "resume", "prompt": "hi"}],
    }
    resp = client.post("/generate", json=body)

    assert resp.status_code == 200
    assert "options" not in captured["json"]


def test_empty_system_omits_system_message(monkeypatch):
    captured = {}

    def fake_post(url, json=None, timeout=None):
        captured["json"] = json
        return FakeResponse(200, {"message": {"content": "ok"}})

    monkeypatch.setattr(main.requests, "post", fake_post)

    body = {
        "model": "qwen3:30b-instruct",
        "calls": [{"key": "resume", "system": "", "prompt": "hi"}],
    }
    resp = client.post("/generate", json=body)

    assert resp.status_code == 200
    assert captured["json"]["messages"] == [
        {"role": "user", "content": "hi"}
    ]


def test_one_call_failing_via_http_error_leaves_other_result(monkeypatch):
    def fake_post(url, json=None, timeout=None):
        key_prompt = json["messages"][-1]["content"]
        if key_prompt == "bad prompt":
            return FakeResponse(
                404, text='model "llama3" not found'
            )
        return FakeResponse(200, {"message": {"content": "good result"}})

    monkeypatch.setattr(main.requests, "post", fake_post)

    body = {
        "model": "llama3",
        "calls": [
            {"key": "good", "prompt": "good prompt"},
            {"key": "bad", "prompt": "bad prompt"},
        ],
    }
    resp = client.post("/generate", json=body)

    assert resp.status_code == 200
    data = resp.json()
    assert data["results"] == {"good": "good result"}
    assert "bad" in data["errors"]
    assert "404" in data["errors"]["bad"]
    assert "llama3" in data["errors"]["bad"]
    assert "bad" not in data["results"]
    assert "good" not in data["errors"]


def test_one_call_failing_via_top_level_error_field(monkeypatch):
    def fake_post(url, json=None, timeout=None):
        key_prompt = json["messages"][-1]["content"]
        if key_prompt == "bad prompt":
            return FakeResponse(
                200, {"error": 'model "llama3" not found'}
            )
        return FakeResponse(200, {"message": {"content": "good result"}})

    monkeypatch.setattr(main.requests, "post", fake_post)

    body = {
        "model": "llama3",
        "calls": [
            {"key": "good", "prompt": "good prompt"},
            {"key": "bad", "prompt": "bad prompt"},
        ],
    }
    resp = client.post("/generate", json=body)

    assert resp.status_code == 200
    data = resp.json()
    assert data["results"] == {"good": "good result"}
    assert "not found" in data["errors"]["bad"]


def test_timeout_seconds_passed_through_to_requests(monkeypatch):
    captured = {}

    def fake_post(url, json=None, timeout=None):
        captured["timeout"] = timeout
        return FakeResponse(200, {"message": {"content": "ok"}})

    monkeypatch.setattr(main.requests, "post", fake_post)

    body = {
        "model": "qwen3:30b-instruct",
        "timeout_seconds": 42,
        "calls": [{"key": "resume", "prompt": "hi"}],
    }
    resp = client.post("/generate", json=body)

    assert resp.status_code == 200
    assert captured["timeout"] == 42


@pytest.mark.parametrize(
    "body",
    [
        {"calls": [{"key": "resume", "prompt": "hi"}]},
        {"model": "", "calls": [{"key": "resume", "prompt": "hi"}]},
        {"model": "qwen3:30b-instruct", "calls": []},
    ],
)
def test_invalid_request_shapes_return_4xx(monkeypatch, body):
    def boom(*args, **kwargs):
        raise AssertionError("requests.post should not be called")

    monkeypatch.setattr(main.requests, "post", boom)

    resp = client.post("/generate", json=body)

    assert 400 <= resp.status_code < 500


def test_duplicate_keys_return_4xx(monkeypatch):
    def boom(*args, **kwargs):
        raise AssertionError("requests.post should not be called")

    monkeypatch.setattr(main.requests, "post", boom)

    body = {
        "model": "qwen3:30b-instruct",
        "calls": [
            {"key": "resume", "prompt": "a"},
            {"key": "resume", "prompt": "b"},
        ],
    }
    resp = client.post("/generate", json=body)

    assert 400 <= resp.status_code < 500
