# Native Clients

This document is the release/run/verification guide for the shipped native thin-client slice.

The repo does not claim a broader finished mobile product than what is verified here. The current shipped scope is:

- iOS thin-client transport, ink capture, audio capture, and black dialogue surface wiring
- Android thin-client transport, ink capture, foreground audio capture, and black dialogue surface wiring
- Boox device detection, raw drawing, and e-ink refresh hooks on the Android client

Use [`native-clients-plan.md`](native-clients-plan.md) for the architecture decision and source-code anchors. Use this document for setup, run, verification, and documentation honesty.

## Setup and Run

1. Start a Tabura server reachable from the device:

   ```bash
   TMP_ROOT="$(mktemp -d -t tabura-native-XXXXXX)"
   PROJECT_DIR="$TMP_ROOT/project"
   DATA_DIR="$TMP_ROOT/data"
   go run ./cmd/tabura server \
     --project-dir "$PROJECT_DIR" \
     --data-dir "$DATA_DIR" \
     --web-host 0.0.0.0 \
     --web-port 8420 \
     --mcp-host 127.0.0.1 \
     --mcp-port 9420
   ```

2. iOS:

   ```bash
   swift test --package-path platforms/ios
   open platforms/ios/TaburaIOS.xcodeproj
   ```

   Build and run `TaburaIOS` on a device or simulator on the same network. Allow microphone and local-network access when prompted.

3. Android:

   ```bash
   ANDROID_HOME=/home/ert/android-sdk gradle -p platforms/android app:testDebugUnitTest
   gradle -p platforms/android/flow-contracts test
   ANDROID_HOME=/home/ert/android-sdk gradle -p platforms/android app:installDebug
   ```

   Build and run the app on an Android device on the same network. The same APK path is used on Onyx Boox hardware.

4. Boox:

   Use the Android build above on current Onyx hardware. The Boox path is the Android client plus `TaburaBooxDevice.kt`, `TaburaBooxInkSurfaceView.kt`, and the Boox SDK dependencies already wired in the Gradle project.

## Automated Verification

Use these checks before claiming the native slice is working:

1. iOS thin-client contract:

   ```bash
   swift test --package-path platforms/ios
   ```

   This covers dialogue presentation logic, event decoding, transport URL helpers, payload encoding, and the shared flow contract.

2. Android thin-client contract:

   ```bash
   ANDROID_HOME=/home/ert/android-sdk gradle -p platforms/android app:testDebugUnitTest
   gradle -p platforms/android/flow-contracts test
   ```

   This covers dialogue presentation logic, transport URL helpers, payload encoding, Boox detection heuristics, and the shared flow contract.

3. Server/web dialogue companion wiring:

   ```bash
   ./scripts/playwright.sh
   ```

   The Playwright suite includes `tests/playwright/live-dialogue-companion.spec.ts`, which covers the shared black dialogue behavior on the web runtime that the native clients mirror.

The structural tests in `platforms/ios/project_files_test.go` and `platforms/android/project_files_test.go` are regression guards for packaging/layout. They are not completion evidence on their own.

## Manual Verification

Attach current hardware results to the PR or issue when platform hardware is involved.

1. iOS server discovery and transport

   Pass: `_tabura._tcp` discovery finds the server or a manual URL connects, chat history loads, canvas snapshot loads, and live chat events continue after connect.

   Fail: discovery never resolves, connect succeeds without chat/canvas data, or websocket updates stop after the first turn.

2. iOS ink, audio, and dialogue surface

   Pass: ink strokes commit to the active chat session, `Black` idle surface enters the full-screen black panel, a tap starts then stops audio capture, and returning from background keeps the app responsive for the next turn.

   Fail: ink is only local, dialogue mode is only cosmetic, recording cannot be stopped cleanly, or background/foreground transition breaks the next capture cycle.

3. Android discovery, transport, ink, and foreground audio

   Pass: the Android client discovers or connects to the server, canvas/chat stay live, stylus or touch input produces `ink_stroke` messages, and the foreground microphone service starts and stops in sync with the dialogue surface.

   Fail: the client loads only static content, ink never reaches the server, or service state diverges from the UI recording state.

4. Boox raw drawing and e-ink refresh

   Pass: the app reports `Boox E-Ink mode active`, raw stylus drawing uses the Onyx path, new canvas content refreshes without stale ghosting, and the content WebView applies the Boox contrast/update hooks.

   Fail: the generic Android ink path is used on Boox hardware, raw drawing never opens, or the screen keeps stale canvas frames after updates.

## Documentation Honesty

Do not describe the native clients as a broader completed product unless the automated checks above pass and the manual checklist above has current hardware results attached.

The current repo claim is limited to the shipped thin-client slice documented here and in `native-clients-plan.md`.
