# Streaming AI Interviewer Deployment Runbook

## Target

Deploy the Go model/orchestrator service and backend relay close to browser users. Brain C runs separately and defaults to the current ngrok endpoint: `https://your-brain-c-ngrok-url.ngrok-free.app`.

For one concurrent interview:

- 2-4 vCPU
- 2-4 GB RAM for local services
- Low-latency egress to the Brain C ngrok region
- Same host or same private network between relay and model service

## Local Dry Run

From `agents/sre/output`:

```bash
docker compose config
docker compose up --build
```

Mock-only contract run:

```bash
BRAIN_C_MODE=mock docker compose up --build
```

Acceptance smoke tests:

```bash
curl http://localhost:8080/healthz
curl http://localhost:3000/healthz
cd ../mlengineer/output/model-service && go run test_client.go
cd ../backend-relay && go run test_client.go
```

Expected:

- Model ack under 400 ms.
- Relay fallback ack under 400 ms.
- Stream order is `ack`, one or more `token`, `end`.
- In remote mode, model service startup fails fast if Brain C `/health` is unreachable.

## Environment Variables

| Variable | Service | Default | Purpose |
|---|---|---|---|
| `MODEL_ADDR` | model-service | `:8080` | Listen address. |
| `CONTEXT_DB` | model-service | `/tmp/ai-interviewer-context.db` | SQLite memory path. |
| `BRAIN_C_MODE` | model-service | `remote` | Use `remote` for production/demo, `mock` only for local contract tests. |
| `BRAIN_C_URL` | model-service | `https://your-brain-c-ngrok-url.ngrok-free.app` | Remote Brain C base URL. Override only when the ngrok endpoint changes. |
| `BRAIN_C_API_KEY` | model-service | empty | Optional API key sent as `X-API-Key`. |
| `BRAIN_C_TIMEOUT_SECONDS` | model-service | `60` | HTTP timeout for Brain C requests. |
| `BRAIN_C_MAX_TOKENS` | model-service | `320` | Max chat completion tokens. |
| `BRAIN_C_TEMPERATURE` | model-service | `0.6` | Chat completion temperature. |
| `RELAY_ADDR` | backend-relay | `:3000` | Listen address. |
| `MODEL_WS_URL` | backend-relay | `ws://model-service:8080/ws` | Model service WebSocket URL. |
| `REGISTRY` | deploy script | `registry.example.com/ai-interviewer` | Container registry prefix. |
| `TAG` | deploy script | timestamp | Image tag. |

## Build And Push

```bash
cd agents/sre/output
REGISTRY=your-registry.example.com/ai-interviewer TAG=20260516 ./deploy.sh
```

## Production Notes

- Keep the model service and relay in the same zone.
- Pin ngrok region near the service and Brain C host when possible.
- Brain C serializes generation, so do not route multiple simultaneous chat completions for the same candidate.
- Use `BRAIN_C_API_KEY` before public demos if Brain C enables auth.
- Preserve `candidate_id` from browser to relay to model service so session state and ledger progression remain stable.
- Preserve the optional `language` field end to end.
- Monitor transcripts for wrapper bypass. Brain C chat prompts should start with `Generate`, `Evaluate`, `Rephrase`, or the final analysis shape.

## Monitoring

Track:

- Ack latency p50/p95/p99.
- Brain C request latency p50/p95/p99.
- Brain C healthcheck failures.
- Ledger and softener helper failures.
- Language detection distribution: `en`, `hi`, `hinglish`.
- WebSocket disconnects.
- WebRTC negotiation failures and fallback rate.
- Process RSS for model service and relay.

Alert when:

- Ack p95 exceeds 400 ms for 5 minutes.
- Brain C healthcheck fails.
- Brain C request p95 exceeds the interview tolerance for 5 minutes.
- Relay fallback error rate exceeds 2%.

## Rollback

1. Keep the previous image tag in the deployment system.
2. Drain new sessions from the current relay.
3. Switch `model-service` and `backend-relay` image tags back to the previous known-good tag.
4. Verify `/healthz` endpoints and run both test clients.
5. Re-enable new interview sessions.
