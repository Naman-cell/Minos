# Frontend Integration Plan

This document is the contract between the frontend, backend relay, model service, and remote Brain C.

## Services

- Model service: `http://localhost:18080`
- Backend relay WebSocket: `ws://localhost:3002/ws`
- Brain C: configured only in backend env as `BRAIN_C_URL`

The frontend should not call Brain C directly.

## 1. Create Interview

Create stores candidate context in the model service session and Brain A memory. It does not start the timer and does not call Brain C for the greeting.

```http
POST /interviews
Content-Type: multipart/form-data
```

Fields:

```text
candidate_id=manual_001
candidate_name=Nirav
job_description=Senior backend role requiring caching and API reliability
seniority=senior
resume_file=@resume.pdf
```

Response:

```json
{
  "session_id": "session_123",
  "candidate_id": "manual_001",
  "duration_seconds": 420,
  "status": "created",
  "start_url": "/interviews/session_123/start",
  "stream_url": "/ws"
}
```

The frontend must store `session_id`. That is the primary key for the rest of the interview.

## 2. Start Interview

Start begins the 7 minute interview timer, resets/creates the Brain C ledger, and calls `/interview/turn` with `transcript: ""` for the opening greeting.

```http
POST /interviews/{session_id}/start
```

Response:

```json
{
  "session_id": "session_123",
  "candidate_id": "manual_001",
  "duration_seconds": 420,
  "response": "Hi Nirav, thanks for joining...",
  "style": "Friendly",
  "status": "not_completed",
  "stream_url": "/ws",
  "session": {
    "phase": "rapport",
    "language": "en",
    "response_style": "Friendly",
    "use_turn_api": true
  }
}
```

Frontend should speak/display `response`. The backend does not synthesize audio.

## 3. Send Candidate Audio

Connect to the backend relay:

```text
ws://localhost:3002/ws
```

Send candidate audio as a base64 WebM clip:

```json
{
  "session_id": "session_123",
  "type": "audio",
  "data": "<base64-webm-audio>",
  "language": "auto"
}
```

Optional:

```json
{
  "candidate_style": "Default"
}
```

The relay sends the audio to Brain C `/transcribe`, forwards the transcript to the model service, then aggregates model-service token frames into one frontend response.

## 4. Receive Interview Response

The frontend receives one complete response object per candidate answer:

```json
{
  "type": "interview_response",
  "session_id": "session_123",
  "response": "Okay, I see. Let me ask...",
  "style": "Friendly",
  "status": "not_completed",
  "language": "en",
  "phase": "rapport"
}
```

Rules:

- `response` is the full constructed text.
- `style` is Brain C's response style.
- `status` is `not_completed` during the interview.
- `status` becomes `completed` after the final closing message.
- The frontend should speak/display `response` directly.

The relay may still send a temporary `filler` frame before the final response while STT is running:

```json
{
  "type": "filler",
  "text": "Okay, I am taking that in.",
  "state": "thinking",
  "language": "en"
}
```

The frontend can show this as a thinking state. It should not treat it as the interviewer answer.

## 5. Time And Completion Behavior

Default interview duration is 420 seconds.

When the interview enters the final minute, the backend lets the current model question and candidate answer complete. The next backend response becomes the final closing message:

```json
{
  "type": "interview_response",
  "response": "Thanks Nirav for joining. It was really nice talking to you, and I hope your interview experience was good. Have a nice day.",
  "style": "Friendly",
  "status": "completed"
}
```

After this, the frontend should stop recording and request the report.

## 6. Fetch Report

Fetch interview/session details:

```http
GET /interviews/{session_id}
```

Response:

```json
{
  "session_id": "session_123",
  "candidate_id": "manual_001",
  "candidate_name": "Nirav",
  "duration_seconds": 420,
  "status": "not_completed",
  "phase": "rapport",
  "language": "en",
  "report_ready": false
}
```

Fetch the report after `status` becomes `completed`:

```http
GET /interviews/{session_id}/report
```

If the report is ready:

```json
{
  "session_id": "session_123",
  "ready": true,
  "report": {},
  "tone_summary": {}
}
```

Manual ending is also supported:

```http
POST /interviews/{session_id}/end
```

## Frontend Checklist

1. Call `POST /interviews` and store `session_id`.
2. Call `POST /interviews/{session_id}/start`.
3. Speak/display the start `response`.
4. Open relay WebSocket.
5. Send every recorded answer with `session_id`.
6. For each `interview_response`, speak/display `response`.
7. Stop the interview when `status === "completed"`.
8. Fetch report using `GET /interviews/{session_id}/report`.
