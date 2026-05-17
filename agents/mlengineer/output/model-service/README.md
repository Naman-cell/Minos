# Model Service

Go WebSocket orchestrator on port `8080` for the streaming Three-Brain AI Interviewer.

This service keeps Brain A and Brain B local in Go, owns candidate session state, wraps candidate turns into Brain C's trained task shapes, calls the remote Brain C server, parses replies, and streams `ack -> token -> end` back to the relay.

## Run

Production remote Brain C mode:

```bash
BRAIN_C_MODE=remote \
BRAIN_C_API_KEY= \
MODEL_ADDR=:8080 \
go run .
```

Local contract testing:

```bash
BRAIN_C_MODE=mock MODEL_ADDR=:18082 go run .
```

The service defaults to Brain C at `https://your-brain-c-ngrok-url.ngrok-free.app`. Override `BRAIN_C_URL` only when the ngrok endpoint changes. The service fails fast in remote mode when Brain C `/health` is unreachable.

## Brain C Contract

The orchestrator never sends raw candidate fragments directly to Brain C chat. It uses:

- `WrapGenerate(topic, level, tags)` -> `Generate a {level}-level interview question about: {topic}.`
- `WrapEvaluate(question, answer)` -> `Evaluate this candidate answer...`
- `WrapRephrase(question)` -> `Rephrase the following interview question...`
- `WrapAnalysis(transcript)` or `POST /analyze`

Remote chat uses:

```http
POST /v1/chat/completions
```

with `model: "customllm"` and OpenAI-style `messages`. Replies are parsed for `<filler>`, `<response>`, and `Score: X/10`.

Helper endpoints used by the orchestrator:

- `GET /health`
- `POST /ledger`
- `GET /ledger/{candidate_id}/next`
- `POST /ledger/{candidate_id}/record`
- `POST /ledger/{candidate_id}/end`
- `POST /softeners/pick`
- `POST /analyze`
- `POST /native-audio-chat` when relay audio is routed to Brain C native audio

## Interview Lifecycle

Start a default 7 minute interview with a resume file:

```http
POST /interviews
Content-Type: multipart/form-data
```

```bash
curl -X POST http://localhost:8080/interviews \
  -F "candidate_id=candidate-001" \
  -F "resume_file=@/path/to/resume.pdf" \
  -F "job_description=Senior backend role requiring distributed systems, caching, observability, and incident response." \
  -F "seniority=senior" \
  -F "duration_seconds=420" \
  -F "language=en"
```

Supported resume file types are `.pdf` and `.docx`. Text-only `resume_text` JSON remains available for tests, but file upload is the production path.

The service extracts resume text, creates/resets the Brain C ledger, stores resume/JD in Brain A session state, generates the first Brain C question, and returns:

```json
{
  "candidate_id": "candidate-001",
  "duration_seconds": 420,
  "first_question": "...",
  "stream_url": "/ws"
}
```

During the interview, send candidate turns over `/ws` with the same `candidate_id`.

End and store the final Brain C report:

```http
POST /interviews/candidate-001/end
```

Fetch the stored report:

```http
GET /interviews/candidate-001/report
```

## WebSocket Input

```json
{
  "candidate_id": "candidate-001",
  "text": "I used Redis with a 5 minute TTL.",
  "context": "Senior backend role requiring API reliability.",
  "language": "auto|en|hi|hinglish"
}
```

## WebSocket Output

```json
{ "type": "ack", "text": "...", "state": "thinking" }
{ "type": "token", "text": "...", "state": "speaking" }
{ "type": "end", "state": "listening" }
```

## Test Client

```bash
go run test_client.go
go run test_client.go -text "Maine cache lagaya tha kyunki database queries slow ho rahi thi."
go run test_client.go -text "Deployment ke baad rollback plan ready tha, but metrics initially unstable the."
go run test_client.go -text "ye maine nhi soccha h abhi tkk."
```

## Tests

```bash
GOCACHE=/private/tmp/go-build go test ./...
```
