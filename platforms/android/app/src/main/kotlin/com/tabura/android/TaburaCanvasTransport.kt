package com.tabura.android

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import org.json.JSONObject

class TaburaCanvasTransport(
    private val client: OkHttpClient,
    private val onArtifact: (TaburaCanvasArtifact) -> Unit,
    private val onDisconnect: (String) -> Unit,
) {
    private var socket: WebSocket? = null

    fun connect(baseUrl: String, sessionId: String) {
        disconnect()
        val request = Request.Builder()
            .url(taburaWsUrl(baseUrl, "canvas/$sessionId"))
            .build()
        socket = client.newWebSocket(request, object : WebSocketListener() {
            override fun onMessage(webSocket: WebSocket, text: String) {
                val payload = runCatching { JSONObject(text) }.getOrNull() ?: return
                onArtifact(parseCanvasArtifact(payload))
            }

            override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                onDisconnect(t.message ?: "canvas transport failed")
            }

            override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
                onDisconnect(reason.ifBlank { "canvas transport closed" })
            }
        })
    }

    fun disconnect() {
        socket?.close(1000, "closing")
        socket = null
    }

    suspend fun loadSnapshot(baseUrl: String, sessionId: String): TaburaCanvasArtifact? {
        return withContext(Dispatchers.IO) {
            val request = Request.Builder()
                .url(taburaApiUrl(baseUrl, "canvas/$sessionId/snapshot"))
                .build()
            client.newCall(request).execute().use { response ->
                val body = response.body?.string().orEmpty()
                if (!response.isSuccessful || body.isBlank()) {
                    return@withContext null
                }
                parseCanvasSnapshot(body)
            }
        }
    }
}
