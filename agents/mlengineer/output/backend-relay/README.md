# Backend Relay

Go relay service on port `3000`.

## Endpoints

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/offer` | Creates a Pion WebRTC offer. |
| `POST` | `/answer` | Accepts the browser SDP answer. |
| `POST` | `/ice-candidate` | Placeholder ICE endpoint. |
| `WS` | `/ws` | Fallback transport with the same browser message schema. |

## Run

```bash
MODEL_WS_URL=ws://localhost:8080/ws STT_MODE=brainc go run .
```

Fallback WS accepts:

```json
{ "type": "audio", "data": "base64 or text:transcript" }
```

Optional language hint:

```json
{ "type": "audio", "data": "base64 or text:transcript", "candidate_id": "candidate-001", "language": "auto|en|hi|hinglish" }
```

Text-only testing is also supported:

```json
{ "text": "I used Redis with a 5 minute TTL.", "candidate_id": "candidate-001", "language": "auto" }
```

It returns the model stream:

```json
{ "type": "ack", "text": "...", "state": "thinking" }
{ "type": "token", "text": "...", "state": "speaking" }
{ "type": "end", "state": "listening" }
```

## STT

Default `STT_MODE=brainc` sends audio chunks to Brain C native audio:

```http
POST /native-audio-chat
```

Browser audio should be sent as a normal encoded clip, for example `webm` from `MediaRecorder`, base64 encoded in the `data` field. The relay posts that clip to Brain C with a strict transcription prompt and forwards the returned text into Brain A/B orchestration.

The native audio endpoint must be enabled on Brain C with `ENABLE_NATIVE_AUDIO=1`. The first audio call can be slow while the multimodal model lazy-loads.

For local text-only tests, the relay still recognizes `text:` payloads and bypasses STT:

```json
{ "type": "audio", "data": "text:I used Redis with a TTL.", "candidate_id": "candidate-001" }
```

Use `STT_MODE=mock` only for isolated relay tests.
