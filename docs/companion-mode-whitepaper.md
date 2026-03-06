# Companion Mode Whitepaper

> **Legal notice:** Tabura is provided "as is" and "as available" without warranties, and to the maximum extent permitted by applicable law the authors/contributors accept no liability for damages, data loss, or misuse. You are solely responsible for backups, verification, and safe operation. See [`DISCLAIMER.md`](/DISCLAIMER.md).

## Summary

Tabura's active direction is a single assistant surface: **Companion Mode**.

Companion Mode replaces the split between conversation mode, participant mode,
and meeting-specific assistant planning. It is intended for:

- live in-person meetings
- one-on-one conversations
- solo workday assistant presence
- online calls as one additional context source

## Core Principles

- **Botless**: no assistant attendee joins Zoom/Meet/Teams
- **Local-first**: Tabura owns capture, buffering, state, and UI locally
- **Whisper-backed**: default STT path remains the `voxtype` Whisper sidecar
- **Always-transcribing for context**: transcript context is continuous while the mode is active
- **Manual control**: users explicitly enter and leave Companion Mode
- **Project-scoped**: context and work belong to projects, not to a global floating agent

## Product Shape

Companion Mode should feel like one humanoid assistant, not multiple separate
features:

- meetings are project sessions
- long-running tasks are project runs
- Hub remains for ad hoc requests and run monitoring
- each project keeps one active run in its main thread
- meetings and long-running jobs default to temporary projects

If no document is displayed, the idle surface is a full-screen minimal humanoid
face. A black-screen idle mode is the alternate surface.

## Persistence Model

- audio remains RAM-only
- text artifacts are persisted
- meetings and long tasks can be persisted or discarded explicitly
- persisted artifacts include transcript text, summaries, references, and run outputs

## Architectural Consequences

- no private repo is required
- no extension/plugin product architecture is required
- new product behavior belongs in the public `krystophny/tabura` repo
- product modularity should come from normal `internal/` package boundaries

## Research Basis

The planned direction is informed by current commercial and open systems:

- Cluely: botless local capture and live assistance during calls
- Granola: meeting-native transcript and summary workflows without attendee bots
- Read Ada / Otter: transcript, summary, cross-session follow-up, and assistant framing
- OpenAI Realtime / LiveKit / Pipecat: low-latency turn handling, interruption, and streaming state
- Tolan: voice-first assistant presence with a clear persona and simple visual state

Tabura should borrow the best parts of those systems without copying their
cloud-recorder assumptions. The target is one public, project-scoped,
Whisper-backed companion surface for live meetings, online calls, and ambient
workday help.

## Research References

- Cluely: <https://cluely.com/>
- Granola: <https://www.granola.ai/>
- Otter: <https://otter.ai/>
- Read Ada: <https://support.read.ai/hc/en-us/articles/49436447541907-Get-started-with-Ada-Read-AI-s-Executive-Assistant>
- OpenAI Realtime: <https://platform.openai.com/docs/guides/realtime-model-capabilities>
- LiveKit turn detector: <https://docs.livekit.io/agents/logic-structure/turns/turn-detector/>
- Pipecat: <https://docs.pipecat.ai/guides/features/gemini-multimodal-live>
- Tolan: <https://openai.com/index/tolan>

## Public Tracking

- Umbrella: `#128`
- Tracker: `#119`
- Directed-speech gate: `#129`
- Response execution: `#130`
- Interaction policy: `#131`
- Memory/timeline: `#132`
- Runtime protocol: `#133`
- Temporary projects: `#134`
- Hub run monitor: `#135`
- Humanoid idle surface / black mode: `#136`
- Transcript memory/context builder: `#137`
- Consent/privacy safeguards: `#138`
