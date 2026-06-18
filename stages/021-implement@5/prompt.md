Goal: Implement Phase 3.2 from PROGRESS.md: create services/ibkr_trading.go to implement interfaces.TradingService read-only paths (account, positions). Stub PlaceOrder and CancelOrder as not implemented to satisfy the compiler.

## Completed stages
- **analyze**: succeeded
  - Model: gemini-3.1-flash-lite, 81.6k tokens in / 3.3k out
  - Files: services/ibkr_trading.go
- **plan**: succeeded
  - Model: gemini-3.1-flash-lite, 71.4k tokens in / 1.9k out
  - Files: PROGRESS.md, services/ibkr_trading.go
- **approve_plan**: succeeded
- **implement**: failed
- **unit_test**: succeeded
  - Model: gemini-3.1-flash-lite, 51.4k tokens in / 591 out
- **explain_failure**: succeeded
  - Model: claude-opus-4-8, 18.9k tokens in / 4.6k out
- **approve_retry**: succeeded
- **implement**: failed
- **unit_test**: succeeded
  - Model: gemini-3.1-flash-lite, 51.4k tokens in / 591 out
- **explain_failure**: succeeded
  - Model: claude-opus-4-8, 18.9k tokens in / 4.6k out
- **approve_retry**: succeeded
- **implement**: failed
- **unit_test**: succeeded
  - Model: gemini-3.1-flash-lite, 51.4k tokens in / 591 out
- **explain_failure**: succeeded
  - Model: claude-opus-4-8, 18.9k tokens in / 4.6k out
- **approve_retry**: succeeded
- **implement**: failed
- **unit_test**: succeeded
  - Model: gemini-3.1-flash-lite, 51.4k tokens in / 591 out
- **explain_failure**: succeeded
  - Model: claude-opus-4-8, 18.9k tokens in / 4.6k out
- **approve_retry**: succeeded

## Context
- human.gate.approve_plan.answer: A
- human.gate.approve_plan.label: A
- human.gate.approve_plan.question: approve_plan
- human.gate.approve_retry.answer: A
- human.gate.approve_retry.label: A
- human.gate.approve_retry.question: approve_retry
- human.gate.label: A
- human.gate.selected: A


Implement only the approved plan. Follow CLAUDE.md and PROGRESS.md. One testable change. Add unit tests if applicable. **CRITICAL: If you must satisfy a Go interface but some methods are out of scope for the current phase, you MUST provide stub methods that return `fmt.Errorf("not implemented in this phase")`.** If the task involves placing/cancelling live orders (port 4001), STOP and fail — that is out of scope and forbidden.