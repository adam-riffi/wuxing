// Package ai is the inference + agency tool: run model work for a service in two
// modes — one-shot infer and bounded agent sessions — and return a result.
// Generic and domain-blind; prompt, model, mode, schema, and the agent's
// allowlist all come from the caller's cfg.
//
// The agent is a bounded content producer, not an actor: packaged text only,
// never cfgs or code, ending at a proposal a human disposes. Side effects go
// through gated wuxing tools over the bus. Backend: Codex CLI. Implemented in
// Phase 5 (infer), with agent mode following once the interpreter routes calls.
package ai
