# Model Service

Go WebSocket orchestrator for the streaming Three-Brain AI Interviewer.

The current production path uses Brain C's single-call interview API:

```http
POST /interview/turn
```

Brain A and Brain B stay local in Go. Brain C owns greeting, phase routing, classification, language matching, softeners, safety templates, ledger updates, and response text.

## Run

```bash
BRAIN_C_MODE=remote \
USE_INTERVIEW_TURN=true \
BRAIN_C_URL=https://your-brain-c-ngrok-url.ngrok-free.app \
CONTEXT_DB=/private/tmp/ai-interviewer-context.db \
MODEL_ADDR=:8080 \
go run -ldflags="-linkmode=external" .
```

Mock mode:

```bash
BRAIN_C_MODE=mock USE_INTERVIEW_TURN=true MODEL_ADDR=:18082 go run -ldflags="-linkmode=external" .
```

## Brain C Contract

The first interview call sends an empty transcript:

```json
{
  "candidate_id": "candidate-001",
  "transcript": "",
  "job_description": "Senior backend role",
  "seniority": "senior",
  "language_hint": "en",
  "region": "IN",
  "candidate_style": "Default"
}
```

Every candidate turn sends the transcribed answer to the same endpoint. The orchestrator streams `response_text` to the client and stores `phase`, `language`, `candidate_style`, `response_style`, safety state, and final report state locally. `candidate_style` is `"Default"` until a real prosody detector is configured upstream.

Legacy `/v1/chat/completions`, `/softeners/pick`, `/ledger/{id}/record`, and `/ledger/{id}/next` are kept only for `USE_INTERVIEW_TURN=false` fallback testing.

## Interview Lifecycle

Start an interview:

```bash
curl -X POST http://localhost:8080/interviews \
  -F "candidate_id=candidate-001" \
  -F "resume_file=@/path/to/resume.pdf" \
  -F "job_description=Senior backend role requiring distributed systems, caching, observability, and incident response." \
  -F "seniority=senior" \
  -F "duration_seconds=420" \
  -F "language=en"
```

The response keeps the existing `first_question` field for browser compatibility, but the value is now Brain C's rapport/greeting response from `/interview/turn`.

## WebSocket Output

```json
{ "type": "style", "response_style": "Friendly", "language": "en", "phase": "interview" }
{ "type": "token", "text": "...", "state": "speaking", "language": "en" }
{ "type": "end", "state": "listening", "language": "en" }
```

The service only emits text/style data. Product TTS belongs in the frontend; the bundled manual page displays the streamed text without speaking it.

Optional frames:

```json
{ "type": "filler", "text": "Hmm, interesting, let me think about that.", "state": "thinking" }
{ "type": "phase", "phase": "interview", "phase_before": "consent_check" }
{ "type": "report", "ended_reason": "natural", "report": {}, "tone_summary": {} }
```

The relay owns `filler` frames while STT is running. They are not sent to the model service or Brain C.

## Tests

```bash
go test ./...
```
