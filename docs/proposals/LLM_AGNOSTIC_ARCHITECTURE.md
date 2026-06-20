# LLM Agnostic Architecture Plan

Transitioning from a hardcoded Anthropic implementation to an SDK-agnostic framework that supports seamless, dynamic switching between Claude and Gemini is a brilliant architectural move. 

Here is the proposed plan to abstract the AI engine and the estimated effort required.

## Phase 1: Agnostic Core Interfaces
**Difficulty: Medium | Estimated Effort: ~2 hours**

Currently, the Go backend (`autonomous_beat.go` and `agent_tools.go`) directly imports and uses Anthropic-specific structs (`anthropic.MessageParam`, `anthropic.ToolUnionParam`). 
- **Action:** We will strip these out and create a generic `llm` package in Go.
- **Components:** We'll define neutral, platform-agnostic data structures: `GenericMessage`, `GenericTool`, `GenericToolCall`, and `GenericToolResult`.
- **Interface:** We'll define a unified `LLMProvider` interface requiring a single method: `ExecuteTurn(ctx, systemPrompt, genericMessages, genericTools)`.

## Phase 2: Provider Implementations
**Difficulty: Medium | Estimated Effort: ~3 hours**

We will implement the `LLMProvider` interface for both AI companies.
- **Anthropic Provider:** We will refactor `services/anthropic_client.go` to serve as a wrapper that translates our `GenericMessage` into Anthropic's SDK format, executes the API call, and translates the response back to generic.
- **Gemini Provider:** We will install `google.golang.org/genai` and build `services/gemini_client.go`. This wrapper will translate `GenericMessage` into Google's `*genai.Content` and map our generic tools into Google's Function Declarations.

## Phase 3: Dynamic UI Switching & Routing
**Difficulty: Low | Estimated Effort: ~1 hour**

The dashboard UI already allows you to configure models like `google/gemini-3-1-pro` and `anthropic/claude-opus-4-8`.
- **Action:** We will build an `LLMFactory` in the Go backend.
- When an agent is invoked, the factory reads the model string prefix (`google/` vs `anthropic/`), automatically instantiates the correct API client using your `.env` keys, and routes the heartbeat execution through that specific provider.

## Phase 4: Cross-AI Context Sharing
**Difficulty: Hard | Estimated Effort: ~4 hours**

This is where the magic happens. Because we are moving the conversation history in `autonomous_beat.go` to a `GenericMessage` slice, the context is no longer locked to a specific AI.
- **How it works:** If a Claude CEO calls the `jim_rogers` tool to consult a Gemini Stratagem, the Claude CEO outputs a standard text prompt. The backend receives this, spins up the Gemini Provider, passes the text, and Gemini responds. 
- **Shared History:** If we want the sub-agent to have the full context of the session, we can easily pass the entire `[]GenericMessage` array to the Gemini provider, which will perfectly translate Claude's previous thoughts into a format Gemini natively understands. 

---

## Overall Assessment
**Total Effort Estimation:** **High** (Approx. 1-2 days of development work).
We are essentially rewriting the entire nervous system of the bot. Every tool schema, message block, and execution loop must be decoupled from the Anthropic SDK.

> [!IMPORTANT]
> ## User Review Required
> This is a significant architectural overhaul, but it permanently solves vendor lock-in and allows you to pit Claude and Gemini against each other in the same Tri-Agent swarm.
>
> 1. Does this abstraction model align with your vision?
> 2. For the cross-AI context sharing, is it sufficient for the `jim_rogers` tool to just pass the immediate prompt text between agents, or do you want the sub-agents to receive the *entire* conversation history of the CEO when consulted?
>
> Let me know if you want to green-light this major refactor!
