# First Run: Three-Brain AI Interviewer

This guide starts the local Go orchestrator, opens the manual browser mic page, sends audio to remote Brain C, and streams the interviewer response back as text/style frames.

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
- Brain C `/transcribe` is enabled on the remote machine

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
USE_INTERVIEW_TURN=true \
CONTEXT_DB=/private/tmp/ai-interviewer-manual.db \
BRAIN_C_URL=https://your-brain-c-ngrok-url.ngrok-free.app \
go run -ldflags="-linkmode=external" .
```

Expected output:

```text
brain C mode=remote
model service listening on :18080
```

If `:18080` is busy, choose another port, for example `:18081`, and use that same port in the browser URL and relay `MODEL_WS_URL`.

## Terminal 2: Start Backend Relay

From the repo root:

```bash
cd agents/mlengineer/output/backend-relay

RELAY_ADDR=:3002 \
MODEL_WS_URL=ws://localhost:18080/ws \
STT_MODE=brainc \
BRAIN_C_URL=https://your-brain-c-ngrok-url.ngrok-free.app \
go run -ldflags="-linkmode=external" .
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
8. Wait for the streamed answer. The manual page displays the response text.
9. Repeat recording answers until done.
10. Click **End interview** to generate and store the Brain C final report.

## What To Expect

On interview start, the model service:

- extracts resume text if a file was uploaded
- calls Brain C `/interview/turn` with `transcript=""` for the greeting/rapport opener
- stores the session locally

On every spoken answer:

- the browser records a WebM audio clip
- relay may send a short local `filler` frame while transcription runs
- relay sends the clip to Brain C `/transcribe`
- relay forwards the returned transcript to the model service
- model service sends the transcript to Brain C `/interview/turn`
- Brain C handles phase routing, language matching, softeners, safety, ledger updates, and response text
- browser displays the streamed text; product speech/TTS is handled by the frontend

## Common Problems

### `bind: address already in use`

Another process is already using the port.

Use different ports:

```bash
MODEL_ADDR=:18081 USE_INTERVIEW_TURN=true go run -ldflags="-linkmode=external" .
```

and:

```bash
RELAY_ADDR=:3003 MODEL_WS_URL=ws://localhost:18081/ws go run -ldflags="-linkmode=external" .
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
BRAIN_C_URL=https://your-brain-c-ngrok-url.ngrok-free.app
```

### Browser records but no response comes back

Check:

- relay is running
- manual page Relay WebSocket URL matches relay port
- Brain C remote server supports `/transcribe`

### Browser shows text but does not speak

That is expected for this repo's manual page. The local services only forward text/style frames; the product frontend owns speech/TTS.

### macOS `missing LC_UUID load command`

If a teammate sees:

```text
dyld: missing LC_UUID load command
signal: abort trap
```

they are likely using an older Go toolchain on a newer macOS version.

Best fix:

```bash
go version
brew update
brew upgrade go
go version
```

Use Go 1.24 or newer.

Quick workaround: add `-ldflags=-linkmode=external` to both `go run` commands.

Model service:

```bash
MODEL_ADDR=:18080 \
BRAIN_C_MODE=remote \
USE_INTERVIEW_TURN=true \
CONTEXT_DB=/private/tmp/ai-interviewer-manual.db \
BRAIN_C_URL=https://your-brain-c-ngrok-url.ngrok-free.app \
go run -ldflags=-linkmode=external .
```

Backend relay:

```bash
RELAY_ADDR=:3002 \
MODEL_WS_URL=ws://localhost:18080/ws \
STT_MODE=brainc \
BRAIN_C_URL=https://your-brain-c-ngrok-url.ngrok-free.app \
go run -ldflags=-linkmode=external .
```

If that fails because `clang` is missing:

```bash
xcode-select --install
```

## Current Limitations

- The reliable local path is the manual page fallback WebSocket using `MediaRecorder` WebM audio and Brain C `/transcribe`.
- The true WebRTC RTP media-track path is not production-ready yet because it still needs proper Opus/RTP decoding or a valid audio container before calling Brain C.
- `/native-audio-chat` is not used for production interviews because it can return multimodal model replies instead of transcript text.
- The bundled manual page is text-only. Production speech/TTS should live in the frontend.

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
