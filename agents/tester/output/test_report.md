# Test Report: `/interview/turn` Migration

Date: 2026-05-18

## Summary

Local Go tests pass for the rebuilt model service and backend relay. The model service defaults to `USE_INTERVIEW_TURN=true`, uses Brain C `POST /interview/turn` for the opening greeting and per-turn interviewer response, and now records latency spans for the opening turn and candidate turns.

Remote Brain C live validation is currently blocked: the configured ngrok URL returns `ERR_NGROK_8012`, meaning ngrok is reachable but its upstream service at `localhost:8000` is refusing the connection.

## Commands Run

Model service:

```bash
cd agents/mlengineer/output/model-service
go test ./...
```

Result:

```text
ok ai-interviewer/model-service
```

Backend relay:

```bash
cd agents/mlengineer/output/backend-relay
go test ./...
```

Result:

```text
ok ai-interviewer/backend-relay
```

Remote Brain C check:

```bash
curl -X POST https://your-brain-c-ngrok-url.ngrok-free.app/interview/turn ...
```

Result:

```text
ERR_NGROK_8012: upstream web service at localhost:8000 refused the connection
```

## Coverage Added

- Start interview calls Brain C `InterviewTurn` with `transcript:""` and returns that greeting in the existing `first_question` field for browser compatibility.
- Start interview sends the opening `InterviewTurn` before any mic/audio turn, with dynamic `candidate_name` and `candidate_style:"Default"`, allowing Brain C's `path=opening_template` fast path to fire.
- Start interview resets the Brain C ledger first, so reused candidate ids such as `manual_001` do not resume stale phase/topic state.
- WebSocket candidate turns in the new path do not emit legacy hardcoded `ack` frames.
- Model service unmarshals optional Brain C `debug_timings` and logs `interview_turn_latency` / `ws_emit_latency` spans for English, Hindi, and Hinglish diagnosis.
- Relay `STT_MODE=brainc` posts browser audio to Brain C `/transcribe`, not `/native-audio-chat`.
- `/native-audio-chat` requires explicit `STT_MODE=native_audio` and is not used for production interviews.
- Relay preserves `phase` and `phase_before` fields instead of emitting half-empty phase frames.
- Relay emits optional varied `filler` frames before binary audio STT to mask latency; filler text is not sent to the model service or Brain C.
- Relay runs prosody detection in parallel with STT and logs `relay_turn_latency` with STT, model, and total spans.
- Mock Brain C implements the typed `/interview/turn` response contract.
- Relay stream test now expects `token -> end`, with optional `phase`/`report` frames allowed by the model service.

## Remaining Blocker

```json
[
  {
    "file": "remote Brain C service",
    "line": 0,
    "severity": "blocker",
    "description": "Configured ngrok tunnel is up, but upstream localhost:8000 is refusing connections. Restart Brain C behind ngrok and rerun /health plus /interview/turn live checks."
  }
]
```
