package com.tabura.android

import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener

class TaburaChatTransport(
    private val client: OkHttpClient,
    private val onEvent: (TaburaChatEventPayload) -> Unit,
    private val onDisconnect: (String) -> Unit,
) {
    private var socket: WebSocket? = null

    fun connect(baseUrl: String, sessionId: String) {
        disconnect()
        val request = Request.Builder()
            .url(taburaWsUrl(baseUrl, "chat/$sessionId"))
            .build()
        socket = client.newWebSocket(request, object : WebSocketListener() {
            override fun onMessage(webSocket: WebSocket, text: String) {
                runCatching { parseChatEvent(text) }
                    .onSuccess(onEvent)
            }

            override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                onDisconnect(t.message ?: "chat transport failed")
            }

            override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
                onDisconnect(reason.ifBlank { "chat transport closed" })
            }
        })
    }

    fun disconnect() {
        socket?.close(1000, "closing")
        socket = null
    }

    fun sendJson(payload: String): Boolean {
        val active = socket ?: return false
        return active.send(payload)
    }
}
