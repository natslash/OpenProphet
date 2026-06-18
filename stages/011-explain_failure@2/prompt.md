Goal: 

## Completed stages
- **analyze**: succeeded
  - Model: gemini-3.1-flash-lite, 98.8k tokens in / 2.0k out
  - Files: PROGRESS.md, services/ibkr_data.go
- **plan**: succeeded
  - Model: gemini-3.1-flash-lite, 111.3k tokens in / 1.6k out
  - Files: services/ibkr_data.go
- **approve_plan**: succeeded
- **implement**: failed
- **unit_test**: succeeded
  - Model: gemini-3.1-flash-lite, 110.8k tokens in / 2.4k out
  - Files: services/ibkr_data.go, tws/tws_client.go
- **explain_failure**: succeeded
  - Model: claude-opus-4-8, 14.1k tokens in / 3.1k out
- **approve_retry**: succeeded
- **implement**: failed
- **unit_test**: succeeded
  - Model: gemini-3.1-flash-lite, 110.8k tokens in / 2.4k out
  - Files: services/ibkr_data.go, tws/tws_client.go

## Context
- human.gate.approve_plan.answer: A
- human.gate.approve_plan.label: A
- human.gate.approve_plan.question: approve_plan
- human.gate.approve_retry.answer: A
- human.gate.approve_retry.label: A
- human.gate.approve_retry.question: approve_retry
- human.gate.label: A
- human.gate.selected: A


Analyze the test failure or review rejection. Explain clearly to the user what went wrong, and outline exactly what different approach the agent will take in the next implementation attempt to fix it.