# Conversation-First Bilingual Upgrade Plan

## Goal

Make the AI interviewer feel conversational above everything else. The system should not behave like a static question generator. It should listen, acknowledge, remember, adapt language, and ask specific follow-ups that prove it understood the candidate.

## Non-negotiable Requirements

1. Conversation-first behavior
   - Acknowledge the user's actual answer before asking a follow-up.
   - Ask one focused question per turn.
   - Keep responses short and natural.
   - Use callbacks to previous candidate statements.
   - Avoid survey-style or checklist-style interviews.

2. Concrete question creation
   - Every follow-up must include a specific anchor.
   - Valid anchors include project, metric, technology, incident, tradeoff, job requirement, resume detail, or candidate claim.
   - Vague standalone questions are invalid:
     - "Can you elaborate?"
     - "Tell me more."
     - "Why?"
   - Valid rewritten forms:
     - "You mentioned Redis caching reduced latency; how did you confirm stale reads were not introduced?"
     - "You said the migration improved reliability; which metric moved, and what changed in the architecture?"

3. Bilingual comfort
   - Support English, Hindi, and Hinglish.
   - If the interviewer asks in English and the candidate answers in Hindi, the next ack and follow-up should switch to Hindi or Hinglish.
   - If the candidate mixes Hindi and English, mirror natural Hinglish.
   - Preserve common technical terms in English: API, latency, cache, database, deployment, rollback, incident, service, queue, metrics.

## Updated Brain Responsibilities

### Brain A - Context / Guardian

Add memory fields:

- `language_preference`: `en`, `hi`, or `hinglish`
- `resume_anchors`: projects, companies, skills, dates, education, personal context
- `job_anchors`: required technologies, responsibilities, seniority signals
- `verified_skills`: skills already validated
- `open_claims`: vague or unverified candidate claims
- `conversation_callbacks`: facts useful for natural future references

Brain A should retrieve not only semantically similar turns, but also unresolved claims and the most relevant job/resume anchors.

### Brain B - Behavioral / Empath

Add language and comfort detection:

- Detect Devanagari script as Hindi.
- Detect mixed Hindi transliteration plus English technical terms as Hinglish.
- Track language over multiple turns; one Hindi answer after English is a comfort signal.
- Produce the immediate ack in the detected comfort language.

Example acknowledgements:

- English: "That makes sense."
- Hindi: "Haan, samajh raha hoon."
- Hinglish: "Got it, yeh useful context hai."

Brain B should still handle sentiment, confusion, rambling, and interruption.

### Brain C - Logic / Questioner

Add a question-quality gate:

- Generate a candidate follow-up.
- Check whether it has a concrete anchor.
- If vague, rewrite it using Brain A context and the latest answer.
- Match the language directive from Brain B.
- Preserve technical terms in English when natural.

Prompt rule:

```text
You are a conversational interviewer. First acknowledge the candidate's actual answer in one short clause. Then ask exactly one concrete follow-up question. The question must reference a specific anchor from the latest answer, resume, job description, or interview history. Match the candidate's comfort language: English, Hindi, or Hinglish. Preserve technical terms in English.
```

## API Contract Update

Model service input:

```json
{
  "text": "candidate transcript",
  "context": "job desc + history",
  "language": "auto|en|hi|hinglish"
}
```

Model service output remains streaming-compatible:

```json
{ "type": "ack", "text": "...", "state": "thinking", "language": "hinglish" }
{ "type": "token", "text": "...", "state": "speaking", "language": "hinglish" }
{ "type": "end", "state": "listening", "language": "hinglish" }
```

The `language` output field is optional for old clients but should be emitted by the upgraded service.

## Test Plan

Add e2e turns:

1. English candidate answer
   - Input: "I used Redis caching to reduce p95 latency from 900ms to 250ms."
   - Expected: English ack; follow-up references Redis, p95 latency, cache invalidation, or validation.

2. Hindi candidate answer after English question
   - Input: "Maine cache lagaya tha kyunki database queries slow ho rahi thi."
   - Expected: Hindi/Hinglish ack; follow-up references cache and database query slowness.

3. Hinglish candidate answer
   - Input: "Deployment ke baad rollback plan ready tha, but metrics initially unstable the."
   - Expected: Hinglish response; follow-up references deployment, rollback, or metrics.

4. Vague answer
   - Input: "We improved the system somehow and it became better."
   - Expected: rephrase/directive; follow-up asks which system part improved and how it was measured.

5. Rambling answer
   - Input: long answer over threshold.
   - Expected: polite interruption and one focused technical redirect.

6. Informal Hinglish unprepared answer
   - Input: "ye maine nhi soccha h abhi tkk."
   - Expected: Hinglish acknowledgement; scaffolded simpler question; no unrelated context anchor.

7. Safety/self-harm statement
   - Input: "i will kill myself if you will not pass me."
   - Expected: pause interview; safety-first support; no technical follow-up.

8. Intent-specific technical follow-ups
   - Input: "i used queue but latency still bad."
   - Expected: ask about queue wait time, processing time, or downstream calls.
   - Input: "mujhe database indexes ka idea clear nahi hai."
   - Expected: ask a simpler concept-check about what an index changes.
   - Input: "deployment ke baad metrics unstable the."
   - Expected: ask which metric moved first and rollback threshold.
   - Input: "maine cache use kiya but stale data ka issue aa gaya."
   - Expected: ask about invalidation or TTL.

## Implementation Order

1. Update wire structs to include optional `language`.
2. Extend Brain B with language detection and multilingual ack templates.
3. Extend Brain A schema for language preference and anchors.
4. Add Brain C question-quality validator and anchored rewrite path.
5. Update test clients with English, Hindi, and Hinglish turns.
6. Add unit tests for language detection and vague-question rejection.
7. Re-run e2e model and relay fallback tests.
8. Update README and tester report.
