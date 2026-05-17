# Minos

Minos is a local prototype of a streaming **Three-Brain AI Interviewer**.

It runs a browser-based interview page, records candidate answers, sends audio to a relay for transcription, passes the transcript through a local interview orchestrator, calls a remote Brain C model for generation/evaluation, and streams the interviewer response back to the browser.

## What You Can Run Locally

- **Model service**: local Go service for Brain A + Brain B orchestration.
- **Backend relay**: local Go service that receives browser audio/text and forwards model responses.
- **Manual browser page**: local test UI at `/manual-test`.
- **Remote Brain C**: expected to run outside this repo behind an ngrok URL.

The most reliable local flow today is the manual browser page using WebSocket fallback audio, not the full WebRTC RTP path.

## Requirements

Install these first:

1. Go 1.22 or newer.
2. A modern browser with microphone access.
3. Optional: Docker and Docker Compose, if you want to run with containers.
4. Access to a running Brain C server.

Default Brain C URL used by the project:

```bash
https://your-brain-c-ngrok-url.ngrok-free.app
```

Check that Brain C is alive:

```bash
curl https://your-brain-c-ngrok-url.ngrok-free.app/health
```

If this fails, ask whoever is running Brain C for the current ngrok URL and use it as `BRAIN_C_URL`.

## Quick Start

Open two terminal windows from the project root.

### 1. Start The Model Service

Terminal 1:

```bash
cd agents/mlengineer/output/model-service

MODEL_ADDR=:18080 \
BRAIN_C_MODE=remote \
CONTEXT_DB=/private/tmp/ai-interviewer-manual.db \
go run .
```

Expected output:

```text
brain C mode=remote
model service listening on :18080
```

If your Brain C ngrok URL changed, include it:

```bash
BRAIN_C_URL=https://your-current-ngrok-url.ngrok-free.app
```

### 2. Start The Backend Relay

Terminal 2:

```bash
cd agents/mlengineer/output/backend-relay

RELAY_ADDR=:3002 \
MODEL_WS_URL=ws://localhost:18080/ws \
STT_MODE=brainc \
BRAIN_C_URL=https://your-brain-c-ngrok-url.ngrok-free.app \
go run .
```

Expected output:

```text
backend relay listening on :3002
```

### 3. Open The Manual Interview Page

Open this in your browser:

```text
http://localhost:18080/manual-test
```

In the page, set **Relay WebSocket URL** to:

```text
ws://localhost:3002/ws
```

Then run the interview:

1. Enter a candidate id, for example `manual_001`.
2. Optionally upload a `.pdf` or `.docx` resume.
3. Paste the job description.
4. Choose seniority and language.
5. Click **Start interview**.
6. Click **Connect relay**.
7. Click **Record answer**, speak, then click again to stop.
8. Wait for the interviewer response.
9. Repeat the answer/response cycle.
10. Click **End interview** to generate the final report.

## Run Without Remote Brain C

Use mock mode when you only want to test the local services.

Terminal 1:

```bash
cd agents/mlengineer/output/model-service
MODEL_ADDR=:18080 BRAIN_C_MODE=mock go run .
```

Terminal 2:

```bash
cd agents/mlengineer/output/backend-relay
RELAY_ADDR=:3002 MODEL_WS_URL=ws://localhost:18080/ws STT_MODE=mock go run .
```

Then open:

```text
http://localhost:18080/manual-test
```

and use:

```text
ws://localhost:3002/ws
```

## Run With Docker Compose

The Docker Compose file lives under the SRE output folder.

```bash
cd agents/sre/output
docker compose up --build
```

This starts:

- model service on `http://localhost:8080`
- backend relay on `http://localhost:3000`

Open:

```text
http://localhost:8080/manual-test
```

Use this relay WebSocket URL in the page:

```text
ws://localhost:3000/ws
```

To use a different Brain C URL:

```bash
BRAIN_C_URL=https://your-current-ngrok-url.ngrok-free.app docker compose up --build
```

To stop everything:

```bash
docker compose down
```

## Test Commands

Model service tests:

```bash
cd agents/mlengineer/output/model-service
go test ./...
```

Backend relay tests:

```bash
cd agents/mlengineer/output/backend-relay
go test ./...
```

## Architecture

The system is split into three main parts.

### Browser Manual Test Page

File:

```text
agents/mlengineer/output/model-service/manual_test.html
```

The browser page is the local test client. It lets you start an interview, upload a resume, record microphone audio, connect to the relay over WebSocket, receive streamed interviewer text, and speak responses using the browser's built-in `speechSynthesis`.

### Backend Relay

Folder:

```text
agents/mlengineer/output/backend-relay
```

The relay is the browser-facing transport layer.

It exposes:

- `WS /ws` for the reliable fallback flow.
- `GET /offer`, `POST /answer`, and `POST /ice-candidate` for WebRTC signalling experiments.

In the reliable local path, the relay receives a browser `MediaRecorder` WebM clip, sends it to Brain C `/native-audio-chat` for speech-to-text, then forwards the transcript to the model service WebSocket.

Important environment variables:

| Variable | Example | Meaning |
|---|---|---|
| `RELAY_ADDR` | `:3002` | Port/address for the relay. |
| `MODEL_WS_URL` | `ws://localhost:18080/ws` | Model service WebSocket URL. |
| `STT_MODE` | `brainc` or `mock` | Whether to use Brain C transcription or local mock text. |
| `BRAIN_C_URL` | `https://...ngrok-free.app` | Remote Brain C server URL. |
| `BRAIN_C_API_KEY` | empty or key | Optional API key sent to Brain C. |

### Model Service

Folder:

```text
agents/mlengineer/output/model-service
```

The model service is the interview orchestrator.

It owns:

- interview start/end endpoints
- candidate session state
- resume text extraction for `.pdf` and `.docx`
- local memory storage in SQLite
- Brain A recall
- Brain B behavior analysis and fast acknowledgements
- Brain C prompt wrapping, scoring, next-question selection, and final report calls
- streaming `ack`, `token`, and `end` messages back over WebSocket

Important endpoints:

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/healthz` | Local health check. |
| `GET` | `/manual-test` | Browser test page. |
| `POST` | `/interviews` | Start an interview. |
| `GET` | `/interviews/{candidate_id}` | Read current session state. |
| `POST` | `/interviews/{candidate_id}/end` | End interview and create final report. |
| `GET` | `/interviews/{candidate_id}/report` | Fetch final report if ready. |
| `WS` | `/ws` | Stream candidate turns and interviewer responses. |

Important environment variables:

| Variable | Example | Meaning |
|---|---|---|
| `MODEL_ADDR` | `:18080` | Port/address for the model service. |
| `CONTEXT_DB` | `/private/tmp/ai-interviewer.db` | SQLite file for local memory. |
| `BRAIN_C_MODE` | `remote` or `mock` | Whether to call remote Brain C or use local mock behavior. |
| `BRAIN_C_URL` | `https://...ngrok-free.app` | Remote Brain C server URL. |
| `BRAIN_C_API_KEY` | empty or key | Optional API key sent to Brain C. |

### Brain A, Brain B, And Brain C

Brain A is local memory. It stores and recalls earlier candidate turns from SQLite so later prompts can include useful context.

Brain B is local behavior logic. It detects simple conversational states such as normal answers, rephrase requests, language choice, and safety-sensitive content. It also provides quick acknowledgements before the slower Brain C response arrives.

Brain C is the remote trained model service. The local orchestrator calls it for interview question generation, answer evaluation, topic ledger updates, softeners, native audio transcription, and final analysis.

## Request Flow

The normal manual interview flow is:

1. Browser opens `http://localhost:18080/manual-test`.
2. Browser starts an interview through the model service `POST /interviews`.
3. Model service creates a candidate session and asks Brain C for the first question.
4. Browser records the candidate answer as WebM audio.
5. Browser sends the audio to backend relay `WS /ws`.
6. Backend relay sends the audio to Brain C `/native-audio-chat` for transcription.
7. Backend relay sends the transcript to model service `WS /ws`.
8. Model service sends a fast Brain B acknowledgement.
9. Model service asks Brain C to evaluate the answer and generate the next question.
10. Model service streams tokens back to the relay.
11. Relay streams them back to the browser.
12. Browser displays and speaks the interviewer response.

## Common Problems

### Port Already In Use

If `:18080` is busy, start the model service on another port:

```bash
MODEL_ADDR=:18081 BRAIN_C_MODE=remote go run .
```

Then start the relay with the matching model URL:

```bash
RELAY_ADDR=:3003 MODEL_WS_URL=ws://localhost:18081/ws STT_MODE=brainc go run .
```

Open:

```text
http://localhost:18081/manual-test
```

and use:

```text
ws://localhost:3003/ws
```

### Brain C Health Check Fails

Check:

```bash
curl https://your-brain-c-ngrok-url.ngrok-free.app/health
```

If the ngrok URL changed, run both services with the new URL:

```bash
BRAIN_C_URL=https://new-url.ngrok-free.app
```

### Browser Records But No Response Returns

Check these items:

- the backend relay is running
- the page uses the correct relay WebSocket URL
- the model service is running
- `MODEL_WS_URL` points to the model service `/ws`
- Brain C supports `/native-audio-chat`
- the first native audio request may be slow while Brain C loads its audio model

### Browser Does Not Speak

Check browser audio permission, microphone permission, and system volume. Local speech output uses browser `speechSynthesis`; there is no separate local TTS service.

## Project Layout

```text
.
├── README.md
├── FIRST_RUN.md
├── agents
│   ├── researcher/output/research_memo.md
│   ├── datascientist/output/simulation.ipynb
│   ├── mlengineer/output/model-service
│   ├── mlengineer/output/backend-relay
│   ├── tester/output/test_report.md
│   └── sre/output/docker-compose.yml
└── models
```

`FIRST_RUN.md` contains the original focused manual-run notes. This README is the broader project guide.

## Current Limitations

- The reliable local path is the manual WebSocket flow using `MediaRecorder` WebM audio.
- The full WebRTC media-track path is experimental.
- Brain C must be running separately for remote mode.
- Local browser speech is test-quality TTS.
- The relay has a mock fallback if the model WebSocket cannot be reached, so check service logs when behavior looks unexpectedly mocked.
