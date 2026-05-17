# Streaming AI Interviewer Research Memo

## Stack Recommendation

Use two Go services plus a remote Brain C inference server.

| Layer | Choice | Reasoning |
|---|---|---|
| Model/orchestrator service | Go 1.22 WebSocket service | Owns Brain A, Brain B, candidate state, task-shaped prompt wrapping, Brain C helper calls, and the `ack -> token -> end` stream contract. |
| Brain C | Remote HTTPS service via `BRAIN_C_URL` | Brain C is already running elsewhere behind ngrok. The orchestrator must call it, not rebuild local llama.cpp. |
| Brain C protocol | OpenAI-style `POST /v1/chat/completions` plus helper endpoints | Matches the trained server contract and lets the orchestrator use ledger, softeners, ASR, interrupt helpers, and analysis. |
| STT | `whisper.cpp` tiny/base adapter in backend relay | Keeps audio handling outside the model service and fits the M2 memory budget. |
| WebRTC | `pion/webrtc` v3 | Pure Go relay with browser audio tracks and data channel streaming. |
| Relay/model transport | WebSocket | Simple browser-adjacent stream for `ack`, `token`, and `end`; easier than gRPC for this hackathon shape. |
| Context memory | SQLite + lightweight embedding adapter | Stores transcript, context, and retrieved memory without a heavy vector service. |
| Behavioral layer | Go rules first, optional FastText later | Keeps immediate acknowledgements under 400 ms and handles English/Hindi/Hinglish comfort signals. |

## Conversation Strategy

The orchestrator owns the live interview. Brain C is task-shaped and should never receive raw candidate fragments as free-form chat.

Each candidate turn follows this order:

1. Brain B detects safety, language comfort, confusion, rambling, or frustration.
2. The service sends a local multilingual ack immediately.
3. Brain A recalls relevant context and updates rolling session memory.
4. If there is a previous question, the orchestrator wraps the candidate answer with `Evaluate this candidate answer...`.
5. It records parsed `Score: X/10` into the Brain C ledger when available.
6. It asks Brain C for the next topic via ledger, then wraps generation as `Generate a {level}-level interview question about: {topic}.`.
7. It parses `<filler>` and `<response>`, then streams the final text over WebSocket.

## Bilingual Strategy

Brain B handles fast language comfort detection in Go:

- Devanagari script maps to Hindi or Hinglish depending on technical English terms.
- Romanized Hindi markers such as `maine`, `nhi`, `socha`, `abhi tak`, `kya`, and `samajh` map to Hinglish when mixed with technical terms.
- Technical terms such as `API`, `cache`, `database`, `latency`, `deployment`, `rollback`, `metrics`, and `queue` remain in English.
- A Hindi/Hinglish answer after an English prompt is treated as a comfort signal, not an error.

Brain C helper softeners can add warmth, but the Go ack remains the latency-critical path.

## Brain-to-Model Mapping

Brain A:

- Go module with SQLite-backed memory.
- Stores transcript, resume/job context, language preference, verified skills, unresolved claims, and concrete anchors.

Brain B:

- Go module with deterministic safety, sentiment, rambling, clarification, and language rules.
- Emits directive plus immediate ack in English, Hindi, or Hinglish.

Brain C:

- Remote specialised inference and helper service.
- Called only through trained task-shaped wrappers: generate, evaluate, rephrase, final analysis.
- Helper endpoints used for ledger progression, softeners, ASR, interrupt phrases, safety checks, and final analysis.

## Latency Analysis

| Step | Expected latency |
|---|---:|
| WebSocket receive/decode | 1-3 ms |
| Brain B behavior + language detection | <10 ms |
| Immediate local ack write | <400 ms target, typically <20 ms |
| Brain A SQLite recall | 2-20 ms |
| Orchestrator -> ngrok edge | 20-100 ms |
| ngrok -> Brain C | 5-20 ms |
| Brain C first token / completion start | 150-400 ms typical |
| Perceived TTFT | <400 ms for ack; remote response follows |

Because Brain C serializes generation, the orchestrator must avoid parallel Brain C generation calls for the same candidate.

## Memory Estimates

| Component | RSS estimate |
|---|---:|
| Model/orchestrator Go service | 30-100 MB |
| Brain A SQLite + embedding adapter | 20-200 MB |
| Brain B rules/FastText optional | 5-50 MB |
| Backend relay + Pion | 50-150 MB |
| whisper.cpp tiny/base | 100-900 MB |
| Session buffers | 20-100 MB |
| Local total excluding remote Brain C | <1.5 GB typical |

## Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| Raw candidate text reaches Brain C chat | Weird non-interviewer responses | Centralize `WrapGenerate`, `WrapEvaluate`, `WrapRephrase`, and `WrapAnalysis`; test wrapper shapes. |
| ngrok URL changes or dies | Interview cannot generate/evaluate | Fail fast in remote mode and report `brain_c_unreachable`; allow `BRAIN_C_MODE=mock` for local tests only. |
| Brain C queues generation | Slow multi-user demos | Keep calls sequential per candidate; avoid parallel chat completions. |
| Warmth becomes repetitive | Robotic feel | Combine local Brain B acks with Brain C softener categories and session state. |
| Language switching sounds unnatural | Candidate discomfort | Mirror comfort language in Brain B and preserve technical terms in English. |
| WebRTC blocked | Audio unavailable | Maintain fallback WebSocket with identical stream schema. |
