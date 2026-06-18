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
  - Model: gemini-3.1-flash-lite, 65.3k tokens in / 886 out

## Context
- human.gate.approve_plan.answer: A
- human.gate.approve_plan.label: A
- human.gate.approve_plan.question: approve_plan
- human.gate.label: A
- human.gate.selected: A


Analyze the test failure or review rejection. Explain clearly to the user what went wrong, and outline exactly what different approach the agent will take in the next implementation attempt to fix it.