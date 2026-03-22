package com.tabura.android

import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class TaburaModelContractTest {
    @Test
    fun chatEventParsingKeepsDialogueControlFields() {
        val payload = parseChatEvent(
            """
            {
              "type": "action",
              "state": "listening",
              "workspace_path": "/tmp/workspace",
              "action": { "type": "toggle_live_dialogue" }
            }
            """.trimIndent(),
        )

        assertEquals("action", payload.type)
        assertEquals("listening", payload.state)
        assertEquals("/tmp/workspace", payload.workspacePath)
        assertEquals("toggle_live_dialogue", payload.actionType)
    }

    @Test
    fun transportHelpersPreserveThinClientWireShape() {
        assertEquals(
            "http://tabura.local:8420/api/chat/sessions/session-1/history",
            taburaApiUrl("http://tabura.local:8420/", "chat/sessions/session-1/history"),
        )
        assertEquals(
            "ws://tabura.local:8420/ws/chat/sessions/session%201",
            taburaWsUrl("http://tabura.local:8420", "chat/sessions/session 1"),
        )
        assertEquals("{\"policy\":\"dialogue\"}", livePolicyRequest("dialogue"))
    }

    @Test
    fun canvasFallbackEscapesTextAndPrefersInlineHtml() {
        val escaped = parseCanvasSnapshot(
            """
            {"event":{"kind":"text","title":"Canvas","text":"<note>&raw"}}
            """.trimIndent(),
        ) ?: error("expected artifact")
        assertTrue(escaped.html.contains("&lt;note&gt;&amp;raw"))

        val inline = parseCanvasSnapshot(
            """
            {"event":{"kind":"text","title":"Canvas","html":"<p>ready</p>","text":"ignored"}}
            """.trimIndent(),
        ) ?: error("expected artifact")
        assertEquals("<p>ready</p>", inline.html)
    }

    @Test
    fun requestBuildersEmitExpectedCapturePayloads() {
        val companionPatch = JSONObject(companionConfigPatch(companionEnabled = true, idleSurface = "black"))
        assertTrue(companionPatch.getBoolean("companion_enabled"))
        assertEquals("black", companionPatch.getString("idle_surface"))

        val audio = JSONObject(audioPcmMessage(byteArrayOf(1, 2, 3)))
        assertEquals("audio_pcm", audio.getString("type"))
        assertEquals("audio/L16;rate=16000;channels=1", audio.getString("mime_type"))
        assertEquals("AQID", audio.getString("data"))

        val ink = JSONObject(
            inkCommitMessage(
                strokes = listOf(
                    TaburaInkStroke(
                        pointerType = "stylus",
                        width = 2.5f,
                        points = listOf(
                            TaburaInkPoint(
                                x = 1f,
                                y = 2f,
                                pressure = 0.5f,
                                tiltX = 3f,
                                tiltY = 4f,
                                roll = 5f,
                                timestampMs = 6L,
                            ),
                        ),
                    ),
                ),
                requestResponse = false,
            ),
        )
        assertEquals("ink_stroke", ink.getString("type"))
        assertFalse(ink.getBoolean("request_response"))
        assertEquals(1, ink.getInt("total_strokes"))
        val stroke = ink.getJSONArray("strokes").getJSONObject(0)
        assertEquals("stylus", stroke.getString("pointer_type"))
    }

    @Test
    fun booxDetectionAcceptsManufacturerOrSdkSignals() {
        assertTrue(
            shouldTreatAsBooxDevice(
                manufacturer = "Onyx",
                hasOnyxSdkPackage = false,
                hasTouchHelperClass = false,
            ),
        )
        assertTrue(
            shouldTreatAsBooxDevice(
                manufacturer = "Acme",
                hasOnyxSdkPackage = true,
                hasTouchHelperClass = false,
            ),
        )
        assertTrue(
            shouldTreatAsBooxDevice(
                manufacturer = "Acme",
                hasOnyxSdkPackage = false,
                hasTouchHelperClass = true,
            ),
        )
        assertFalse(
            shouldTreatAsBooxDevice(
                manufacturer = "Acme",
                hasOnyxSdkPackage = false,
                hasTouchHelperClass = false,
            ),
        )
    }
}
