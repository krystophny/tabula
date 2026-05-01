# Native Clients Plan

> **Legal notice:** Slopshell is provided "as is" and "as available" without warranties, and to the maximum extent permitted by applicable law the authors/contributors accept no liability for damages, data loss, or misuse. You are solely responsible for backups, verification, and safe operation. See [`DISCLAIMER.md`](/DISCLAIMER.md).

## Architecture Decision

Slopshell's mobile direction is server-driven thin native clients.

Business logic lives in the Go server. Native clients stay focused on platform
I/O, low-latency capture, native rendering, and background/runtime integration.
That keeps behavior fixes centralized in `internal/web/` and `internal/mcp/`
instead of splitting product logic across multiple frontends.

Each native client owns three responsibilities:

- **Capture**: audio PCM, ink strokes, taps, and gestures.
- **Render**: structured chat/canvas output rendered with native surfaces.
- **Platform services**: background audio, push notifications, wake locks, and
  e-ink refresh hooks where applicable.

## Current Server Anchors

The current server already exposes the foundations required by thin native
clients:

- `internal/web/chat_ws.go` for chat websocket turns.
- `internal/web/server_relay.go` for canvas relay and file-backed canvas
  transport.
- `internal/web/mdns.go` for loopback-safe mDNS advertisement of the runtime.
- `internal/web/push.go` for push registration and relay plumbing.
- `tests/playwright/canvas.spec.ts` for render-protocol coverage.

## Platform Surfaces

The shipped native surfaces match the thin-client split.

### iOS

- `platforms/ios/SlopshellIOS/SlopshellInkCaptureView.swift` uses `PencilKit` for ink
  capture.
- `platforms/ios/SlopshellIOS/SlopshellAudioCapture.swift` owns microphone capture.
- `platforms/ios/SlopshellIOS/SlopshellCanvasTransport.swift` and
  `platforms/ios/SlopshellIOS/SlopshellChatTransport.swift` connect to the server.
- `platforms/ios/SlopshellIOS/SlopshellServerDiscovery.swift` handles `_slopshell._tcp`
  discovery.
- `platforms/ios/SlopshellIOS/ContentView.swift` now exposes an explicit native
  dialogue surface selector and a full-screen black dialogue mode.
- `platforms/ios/SlopshellIOS/SlopshellAppModel.swift` wires dialogue entry and exit
  to `/api/live-policy`, `/api/workspaces/{id}/companion/config`, and incoming
  `toggle_live_dialogue` / `companion_state` chat events.

### Android and Boox

- `platforms/android/app/src/main/kotlin/com/slopshell/android/SlopshellInkSurfaceView.kt`
  uses `MotionEventPredictor` for low-latency stylus capture.
- `platforms/android/app/src/main/kotlin/com/slopshell/android/SlopshellBooxInkSurfaceView.kt`
  uses `TouchHelper.create` and raw drawing for Onyx Boox devices.
- `platforms/android/app/src/main/kotlin/com/slopshell/android/SlopshellAudioCaptureService.kt`
  owns background microphone capture.
- `platforms/android/app/src/main/kotlin/com/slopshell/android/SlopshellCanvasTransport.kt`
  and
  `platforms/android/app/src/main/kotlin/com/slopshell/android/SlopshellChatTransport.kt`
  connect to the server.
- `platforms/android/app/src/main/kotlin/com/slopshell/android/SlopshellServerDiscovery.kt`
  handles `_slopshell._tcp` discovery.
- `platforms/android/app/src/main/kotlin/com/slopshell/android/MainActivity.kt`
  now exposes an explicit native dialogue surface selector and a full-screen
  black dialogue mode.
- `platforms/android/app/src/main/kotlin/com/slopshell/android/SlopshellAppModel.kt`
  wires dialogue entry and exit to `/api/live-policy`,
  `/api/workspaces/{id}/companion/config`, and incoming
  `toggle_live_dialogue` / `companion_state` chat events.

### Web

- `internal/web/static/app-runtime-ui.ts` toggles `black-screen` dialogue mode.
- `internal/web/static/companion.css` defines the black-screen runtime surface.
- `tests/playwright/live-dialogue-companion.spec.ts` covers black-screen
  dialogue behavior.

## Ink Latency Targets

The current runtime keeps the original latency targets as design guidance:

| Platform | Target | Technique |
| --- | --- | --- |
| iOS + Apple Pencil | ~9ms | `PencilKit` with native prediction |
| Android + stylus | ~4ms | `MotionEventPredictor` with the Ink stack |
| Onyx Boox e-ink | ~100ms | raw drawing via `TouchHelper` |
| Web + stylus | ~10ms | browser ink path with delegated presentation where available |

These are target envelopes, not CI-enforced benchmarks. The concrete shipped
techniques are anchored in the platform files above.

## Delivery Status

This is not a claim that every possible native-client deployment has been
validated. The current repo claim is limited to the shipped iOS/Android
thin-client slice, the Android Boox code path, and the black-screen dialogue
path documented here and in [`native-clients.md`](native-clients.md).

Dialogue black-screen mode is intentionally implemented across the shipped
clients:

- `#632` server-side render protocol
- `#633` native iOS client
- `#634` native Android client
- `#636` web ink rewrite
- `#637` black-screen dialogue mode on web, iOS, and Android
- `#638` mDNS advertisement and push relay

Boox-specific code paths are implemented in the Android client. Boox release
readiness still requires the hardware evidence listed in
[`native-clients.md`](native-clients.md); do not claim Boox readiness from
generic Android validation alone.

## Verification and Runbook

Release/run/use instructions and the platform verification checklist live in
[`native-clients.md`](native-clients.md).

Treat `platforms/ios/project_files_test.go` and
`platforms/android/project_files_test.go` as packaging regression guards only.
Completion evidence comes from `npm run test:flows:native`, the fast native
contract suites, `npm run test:native-docs`, and the manual hardware checklist.

## Native Dialogue Mode Operation

Native dialogue mode is now explicit instead of implied:

- Choose `Robot` or `Black` in the native dialogue surface control.
- Tap `Start Dialogue` to enter live dialogue locally. The client posts
  `/api/live-policy` with `dialogue` and ensures
  `/api/workspaces/{id}/companion/config` has `companion_enabled=true`.
- When the selected idle surface is `black`, the client swaps into a full-screen
  black tap target and keeps the screen awake while dialogue mode stays active.
- Tap the full-screen surface to start recording and tap again to stop. Android
  continues to use the foreground microphone service for the active recording
  path.
- Tap `Exit Dialogue` or trigger `toggle_live_dialogue` from the server to
  leave the mode.

## Manual Verification

Use these pass/fail checks when real devices are available:

1. iOS black-screen dialogue surface
   Pass: set the surface to `Black`, tap `Start Dialogue`, confirm the app
   enters a full-screen black surface, the screen does not dim, and a tap starts
   then stops microphone capture.
   Fail: the app stays in the standard shell, the screen sleeps, or taps do not
   control recording.
2. Android black-screen dialogue surface
   Pass: set the surface to `Black`, tap `Start Dialogue`, confirm the app
   enters a full-screen black surface, the display stays awake, and a tap starts
   then stops the foreground microphone service path.
   Fail: the app stays in the standard shell, the display sleeps, or recording
   state diverges from the foreground-service state.
3. Boox raw drawing and refresh
   Pass: Boox detection chooses the raw drawing surface, stylus input emits the
   normalized `ink_stroke` payload, canvas content uses e-ink styling, and the
   e-ink controller refresh runs after content load.
   Fail: Boox uses the generic Android surface, raw input stays local, or
   canvas updates require manual full refresh to become readable.
4. Server/client wiring
   Pass: switching the surface updates `/api/workspaces/{id}/companion/config`,
   entering dialogue posts `/api/live-policy`, and a server
   `toggle_live_dialogue` action toggles the native mode.
   Fail: native dialogue mode only works as a local visual toggle with no server
   state integration.
5. Product docs honesty
   Pass: product docs only claim the shipped iOS/Android thin-client slice and point to
   [`native-clients.md`](native-clients.md) for run and verification steps.
   Fail: docs describe the native clients as complete without matching automated
   and hardware evidence.
