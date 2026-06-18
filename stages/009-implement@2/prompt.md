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
  - Model: gemini-3.1-flash-lite, 129.4k tokens in / 1.6k out
  - Files: PROGRESS.md, services/ibkr_data.go
- **explain_failure**: succeeded
  - Model: claude-opus-4-8, 14.1k tokens in / 3.1k out
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


Implement only the approved plan. Follow CLAUDE.md and PROGRESS.md. Execute Phase 3 tasks (wiring up the data and trading services). One testable change. Add unit tests if applicable. **If the task involves placing/cancelling live orders (port 4001), STOP and fail — that is out of scope and forbidden.**