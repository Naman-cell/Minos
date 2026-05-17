# First Run: Three-Brain AI Interviewer

This guide starts the local Go orchestrator, opens the manual browser mic page, sends audio to remote Brain C, streams the interviewer response back, and speaks it in the browser.

## What Runs Where

- **Model service / Brain A+B:** local Go service, default test port `18080`
- **Backend relay:** local Go relay, default test port `3002`
- **Brain C:** remote ngrok server
- **Manual test page:** `http://localhost:18080/manual-test`

Default remote Brain C URL:

```bash
https://your-brain-c-ngrok-url.ngrok-free.app
```

## Prerequisites

- Go installed
- Brain C remote server is running and `/health` is reachable
- Browser microphone permission is allowed
- Brain C native audio is enabled on the remote machine with `/native-audio-chat`

Quick Brain C check:

```bash
curl https://your-brain-c-ngrok-url.ngrok-free.app/health
```

## Terminal 1: Start Model Service

From the repo root:

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

If `:18080` is busy, choose another port, for example `:18081`, and use that same port in the browser URL and relay `MODEL_WS`.

## Terminal 2: Start Backend Relay

From the repo root:

```bash
cd agents/mlengineer/output/backend-relay

RELAY_ADDR=:3002 \
MODEL_WS=ws://localhost:18080/ws \
STT_MODE=brainc \
BRAIN_C_URL=https://your-brain-c-ngrok-url.ngrok-free.app \
go run .
```

Expected output:

```text
backend relay listening on :3002
```

If `:3002` is busy, choose another port, for example `:3003`, and put that port in the manual page Relay WebSocket URL.

## Browser: Run The Manual Interview

Open:

```text
http://localhost:18080/manual-test
```

Set **Relay WebSocket URL** to:

```text
ws://localhost:3002/ws
```

Then:

1. Enter a candidate id, for example `manual_001`.
2. Optionally upload a `.pdf` or `.docx` resume.
3. Enter the job description.
4. Select seniority and language.
5. Click **Start interview**.
6. Click **Connect relay**.
7. Click **Record answer**, speak, then click again to stop.
8. Wait for the streamed answer. The browser speaks the text sentence-by-sentence.
9. Repeat recording answers until done.
10. Click **End interview** to generate and store the Brain C final report.

## What To Expect

On interview start, the model service:

- extracts resume text if a file was uploaded
- creates/resets the Brain C ledger
- asks Brain C for the first question using a task-shaped prompt
- stores the session locally

On every spoken answer:

- the browser records a WebM audio clip
- relay sends the clip to Brain C `/native-audio-chat` with a transcription prompt
- relay forwards the returned transcript to the model service
- Brain B sends a fast local acknowledgement
- model service wraps the transcript as an evaluation prompt for Brain C
- model service records score to the Brain C ledger when available
- model service asks Brain C for the next question
- browser speaks the streamed text sentence-by-sentence

## Common Problems

### `bind: address already in use`

Another process is already using the port.

Use different ports:

```bash
MODEL_ADDR=:18081 go run .
```

and:

```bash
RELAY_ADDR=:3003 MODEL_WS=ws://localhost:18081/ws go run .
```

Then open:

```text
http://localhost:18081/manual-test
```

and set:

```text
ws://localhost:3003/ws
```

### Brain C health check fails

The model service will not start in remote mode if Brain C `/health` is unreachable.

Check:

```bash
curl https://your-brain-c-ngrok-url.ngrok-free.app/health
```

If ngrok changed, set:

```bash
BRAIN_C_URL=https://new-url.ngrok-free.app
```

### Browser records but no response comes back

Check:

- relay is running
- manual page Relay WebSocket URL matches relay port
- Brain C remote server supports `/native-audio-chat`
- first native audio call may be slow while Brain C lazy-loads audio model

### Browser does not speak

Check browser audio permissions and system volume. The manual page uses built-in browser `speechSynthesis`; no external TTS service is required for this local test.

## Current Limitations

- The reliable local speech path is the manual page fallback WebSocket using `MediaRecorder` WebM audio.
- The true WebRTC RTP media-track path is not production-ready yet because it still needs proper Opus/RTP decoding or a valid audio container before calling Brain C.
- `/native-audio-chat` is being prompted to return transcript-only text. If Brain C returns a model reply instead of a transcript, the orchestrator may evaluate the wrong text.
- Browser speech is local test TTS. Production-quality TTS should be added separately.

## Test Commands

Model service:

```bash
cd agents/mlengineer/output/model-service
go test ./...
```

Backend relay:

```bash
cd agents/mlengineer/output/backend-relay
go test ./...
```

