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
- **explain_failure**: succeeded
  - Model: claude-opus-4-8, 60.0k tokens in / 21.9k out
  - Files: /workspace/OpenProphet/PROGRESS.md, /workspace/OpenProphet/cmd/tws_smoke_test/main.go, /workspace/OpenProphet/services/ibkr_trading.go, /workspace/OpenProphet/tws/constants.go, /workspace/OpenProphet/tws/decoder.go, /workspace/OpenProphet/tws/decoder_test.go, /workspace/OpenProphet/tws/encoder.go, /workspace/OpenProphet/tws/tws_client.go, /workspace/OpenProphet/tws/wrapper.go
- **approve_retry**: succeeded
- **implement**: failed

## Context
- failure_class: transient_infra
- failure_signature: implement|transient_infra|api_transient|gemini|rate_limited
- human.gate.approve_plan.answer: A
- human.gate.approve_plan.label: A
- human.gate.approve_plan.question: approve_plan
- human.gate.approve_retry.answer: A
- human.gate.approve_retry.label: A
- human.gate.approve_retry.question: approve_retry
- human.gate.label: A
- human.gate.selected: A


go test -race ./tws/... ./services/...