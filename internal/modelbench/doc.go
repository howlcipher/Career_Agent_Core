// Package modelbench measures how the local Ollama models installed on this
// host perform on a small set of bounded, objectively-validated task classes,
// so model routing decisions (which model handles which kind of work) can be
// made from evidence instead of the assumption that the largest model is
// always the best choice.
//
// It is deliberately independent of pkg/mcp: that package's ollamaProvider is
// the production inference path and intentionally does not surface Ollama's
// per-call timing fields (load/prompt/eval durations) since nothing in the
// product needs them. This package talks to the same Ollama HTTP API
// directly so it can capture that data without changing production code or
// its risk profile.
//
// Everything here is read-only against Ollama and the host: it never pulls,
// deletes, or otherwise mutates a model, and every task fixture is synthetic
// and committed to the repository (see tasks.go) rather than derived from
// real logs, resumes, or application data.
package modelbench
