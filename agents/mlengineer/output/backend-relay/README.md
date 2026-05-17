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
MODEL_WS_URL=ws://localhost:8080/ws STT_MODE=brainc go run -ldflags="-linkmode=external" .
```

Fallback WS accepts:

```json
{ "type": "audio", "data": "base64 or text:transcript" }
```

Optional language hint:

```json
{ "type": "audio", "data": "base64 or text:transcript", "candidate_id": "candidate-001", "language": "auto|en|hi|hinglish", "candidate_style": "Default" }
```

Text-only testing is also supported:

```json
{ "text": "I used Redis with a 5 minute TTL.", "candidate_id": "candidate-001", "language": "auto", "candidate_style": "Default" }
```

It returns the model stream:

```json
{ "type": "style", "response_style": "Friendly", "language": "en", "phase": "interview" }
{ "type": "token", "text": "...", "state": "speaking" }
{ "type": "end", "state": "listening" }
```

When the model service runs with `USE_INTERVIEW_TURN=true`, the relay should not expect a legacy `ack` frame. `style`, optional `phase`, and `report` frames pass through unchanged, including `response_style` and `tone_summary`.

For real binary audio, the relay may emit a local latency-masking frame before STT completes:

```json
{ "type": "filler", "text": "Hmm, interesting, let me think about that.", "state": "thinking" }
```

This filler is never sent to the model service and must not be included in the transcript.

## Style Handoff

The relay has a prosody detector boundary and currently uses `FallbackProsodyDetector`, which always returns `"Default"`. That value is forwarded to the model service as `candidate_style`; Brain C returns `response_style`, and the product frontend can use that value for TTS.

## STT

Default `STT_MODE=brainc` sends browser audio to Brain C transcription:

```http
POST /transcribe
```

Browser audio should be sent as a normal encoded clip, for example `webm` from `MediaRecorder`, base64 encoded in the `data` field. The relay posts that clip to Brain C `/transcribe` and forwards only the returned transcript into the model service. The model service then calls `/interview/turn`.

Do not use `/native-audio-chat` for production interviews. It is a multimodal response endpoint and may not use the interviewer LoRA. It is available only by explicitly setting:

```bash
STT_MODE=native_audio
```

For local text-only tests, the relay still recognizes `text:` payloads and bypasses STT:

```json
{ "type": "audio", "data": "text:I used Redis with a TTL.", "candidate_id": "candidate-001" }
```

Use `STT_MODE=mock` only for isolated relay tests.
