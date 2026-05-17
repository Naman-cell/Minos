# AI Interviewer Remote Brain C Test Report

Status: local contract tests passed; remote Brain C URL is configured by default. Live validation should run against `https://your-brain-c-ngrok-url.ngrok-free.app` when network access is available.

Date: 2026-05-16

## Runtime Shape

The rebuilt system uses:

- Go model service as the orchestrator for Brain A, Brain B, candidate sessions, prompt wrapping, and Brain C helper calls.
- Remote Brain C over `BRAIN_C_URL` in `BRAIN_C_MODE=remote`; default URL is `https://your-brain-c-ngrok-url.ngrok-free.app`.
- Test-only deterministic Brain C in `BRAIN_C_MODE=mock`.
- Backend relay passthrough for `candidate_id`, `text`/audio, and language hint.

## Unit Tests

Model service:

```bash
GOCACHE=/private/tmp/go-build go test ./...
```

Result: passed.

Backend relay:

```bash
GOCACHE=/private/tmp/go-build go test ./...
```

Result: passed when permitted to open a local `httptest` listener.

## Mock Smoke Test

Started model service:

```bash
BRAIN_C_MODE=mock MODEL_ADDR=:18082 CONTEXT_DB=/private/tmp/ai-interviewer-smoke.db go run .
```

Ran:

```bash
go run test_client.go -addr ws://localhost:18082/ws -candidate-id smoke_001 -text "I used Redis with a 5 minute TTL to reduce p95 latency."
```

Observed:

```text
ack in 0ms [en]: That makes sense.
Alright. On Redis caching, can you walk me through one concrete tradeoff you made and the metric that proved it worked?
end: listening
```

## Interview Lifecycle Smoke Test

Started a temporary model service:

```bash
BRAIN_C_MODE=mock MODEL_ADDR=:18083 CONTEXT_DB=/private/tmp/ai-interviewer-lifecycle.db go run .
```

Started a default 7 minute interview. JSON `resume_text` was used for this smoke test; unit tests cover multipart DOCX/PDF resume extraction:

```bash
curl -X POST http://127.0.0.1:18083/interviews \
  -H 'Content-Type: application/json' \
  -d '{"candidate_id":"life_001","resume_text":"Backend engineer with Redis caching experience.","job_description":"Senior backend role requiring caching, API reliability, and incident response.","seniority":"senior","duration_seconds":420,"language":"en"}'
```

Multipart production shape:

```bash
curl -X POST http://127.0.0.1:18083/interviews \
  -F "candidate_id=life_001" \
  -F "resume_file=@resume.docx" \
  -F "job_description=Senior backend role requiring caching, API reliability, and incident response." \
  -F "seniority=senior" \
  -F "duration_seconds=420" \
  -F "language=en"
```

Observed:

```json
{
  "candidate_id": "life_001",
  "duration_seconds": 420,
  "first_question": "Alright. On Redis caching, can you walk me through one concrete tradeoff you made and the metric that proved it worked?",
  "stream_url": "/ws"
}
```

Sent one candidate turn over WebSocket:

```bash
go run test_client.go -addr ws://127.0.0.1:18083/ws -candidate-id life_001 -text "I used Redis with a 5 minute TTL and watched p95 latency drop."
```

Observed evaluation + next question:

```text
ack in 3ms [en]: That makes sense.
That makes sense. Score: 7/10. ... On API reliability, can you walk me through one concrete tradeoff you made and the metric that proved it worked?
end: listening
```

Ended the interview and fetched the stored report:

```bash
curl -X POST http://127.0.0.1:18083/interviews/life_001/end
curl http://127.0.0.1:18083/interviews/life_001/report
```

Observed stored JSON report:

```json
{
  "candidate_id": "life_001",
  "ready": true,
  "report": {
    "confidence": "Medium",
    "recommendation": "Maybe"
  }
}
```

## Covered Checks

- Wrapper shapes:
  - `Generate a {level}-level interview question about: {topic}.`
  - `Evaluate this candidate answer...`
  - `Rephrase the following interview question...`
- Reply parsing:
  - `<filler>`
  - `<response>`
  - `Score: X/10`
- Go session context:
  - candidate id
  - resume/job snippets
  - transcript
  - last question/topic/level
- Local Brain B:
  - English/Hindi/Hinglish detection
  - safety pause
  - clarification and frustration routing
- Relay stream order:
  - `ack -> token -> end`

## Remote Brain C Note

```json
[
  {
    "file": "environment",
    "line": 0,
    "severity": "low",
    "description": "BRAIN_C_URL is now configured by default. If this ngrok tunnel changes or expires, override BRAIN_C_URL or update the default config."
  }
]
```

## Remaining Risks

- Remote Brain C latency over ngrok still needs measurement from this machine.
- Ledger endpoint schema is implemented from the integration plan and should be verified against the live server.
- STT now routes relay audio to Brain C `/native-audio-chat` with a strict transcription prompt. The first real audio call can be slow while Brain C lazy-loads native audio.
