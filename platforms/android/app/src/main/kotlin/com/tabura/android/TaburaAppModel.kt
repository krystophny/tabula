package com.tabura.android

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONObject

class TaburaAppModel(application: Application) : AndroidViewModel(application) {
    data class UiState(
        val serverUrl: String = "http://127.0.0.1:8420",
        val password: String = "",
        val composerText: String = "",
        val messages: List<TaburaRenderedMessage> = emptyList(),
        val canvas: TaburaCanvasArtifact = TaburaCanvasArtifact(
            kind = "",
            title = "",
            html = "<p style=\"margin:24px;font:sans-serif;\">Connect to a Tabura server to load the canvas.</p>",
            text = "",
        ),
        val workspaces: List<TaburaWorkspace> = emptyList(),
        val selectedWorkspaceId: String = "",
        val statusText: String = "Disconnected",
        val lastError: String = "",
        val isRecording: Boolean = false,
        val inkRequestsResponse: Boolean = true,
        val discoveredServers: List<TaburaDiscoveredServer> = emptyList(),
    )

    private val client = OkHttpClient()
    private val jsonMediaType = "application/json".toMediaType()
    private val _state = MutableStateFlow(UiState())
    val state: StateFlow<UiState> = _state.asStateFlow()

    private val discovery = TaburaServerDiscovery(
        context = application.applicationContext,
        onServersChanged = { servers ->
            _state.update { current -> current.copy(discoveredServers = servers) }
        },
        onError = { message -> setError(message) },
    )
    private val chatTransport = TaburaChatTransport(
        client = client,
        onEvent = ::handleChatEvent,
        onDisconnect = { message ->
            _state.update { current -> current.copy(statusText = "Chat disconnected", lastError = message) }
        },
    )
    private val canvasTransport = TaburaCanvasTransport(
        client = client,
        onArtifact = { artifact ->
            _state.update { current -> current.copy(canvas = artifact) }
        },
        onDisconnect = { message ->
            _state.update { current -> current.copy(statusText = "Canvas disconnected", lastError = message) }
        },
    )

    private var activeWorkspace: TaburaWorkspace? = null

    init {
        discovery.start()
    }

    override fun onCleared() {
        chatTransport.disconnect()
        canvasTransport.disconnect()
        discovery.stop()
        super.onCleared()
    }

    fun updateServerUrl(value: String) {
        _state.update { current -> current.copy(serverUrl = value) }
    }

    fun updatePassword(value: String) {
        _state.update { current -> current.copy(password = value) }
    }

    fun updateComposerText(value: String) {
        _state.update { current -> current.copy(composerText = value) }
    }

    fun updateRecordingState(active: Boolean, message: String = "") {
        _state.update { current ->
            current.copy(
                isRecording = active,
                lastError = message.ifBlank { current.lastError },
            )
        }
    }

    fun setInkRequestsResponse(enabled: Boolean) {
        _state.update { current -> current.copy(inkRequestsResponse = enabled) }
    }

    fun useDiscoveredServer(server: TaburaDiscoveredServer) {
        _state.update { current -> current.copy(serverUrl = server.baseUrlString) }
    }

    fun connect() {
        viewModelScope.launch {
            val baseUrl = state.value.serverUrl.trim()
            if (baseUrl.isBlank()) {
                setError("Enter a valid Tabura server URL.")
                return@launch
            }
            runCatching {
                loginIfNeeded(baseUrl)
                val workspaceResponse = loadWorkspaces(baseUrl)
                val selected = workspaceResponse.workspaces.firstOrNull {
                    it.id == workspaceResponse.activeWorkspaceId
                } ?: workspaceResponse.workspaces.firstOrNull()
                _state.update { current ->
                    current.copy(
                        workspaces = workspaceResponse.workspaces,
                        selectedWorkspaceId = selected?.id.orEmpty(),
                    )
                }
                if (selected != null) {
                    attachWorkspace(baseUrl, selected)
                } else {
                    _state.update { current -> current.copy(statusText = "Authenticated") }
                }
            }.onFailure { error ->
                _state.update { current ->
                    current.copy(
                        statusText = "Connection failed",
                        lastError = error.message ?: "Connection failed",
                    )
                }
            }
        }
    }

    fun switchWorkspace(workspaceId: String) {
        val workspace = state.value.workspaces.firstOrNull { it.id == workspaceId } ?: return
        val baseUrl = state.value.serverUrl.trim()
        viewModelScope.launch {
            runCatching {
                attachWorkspace(baseUrl, workspace)
            }.onFailure { error ->
                setError(error.message ?: "Workspace switch failed")
            }
        }
    }

    fun sendComposerMessage() {
        val workspace = activeWorkspace ?: return
        val text = state.value.composerText.trim()
        if (text.isBlank()) {
            return
        }
        viewModelScope.launch {
            runCatching {
                postJson(
                    url = taburaApiUrl(state.value.serverUrl, "chat/sessions/${workspace.chatSessionId}/messages"),
                    body = composerRequest(text),
                )
                _state.update { current ->
                    current.copy(
                        composerText = "",
                        messages = current.messages + TaburaRenderedMessage(
                            id = "local-${System.currentTimeMillis()}",
                            role = "user",
                            text = text,
                        ),
                    )
                }
            }.onFailure { error ->
                setError(error.message ?: "Message send failed")
            }
        }
    }

    fun sendAudioChunk(data: ByteArray) {
        if (!chatTransport.sendJson(audioPcmMessage(data))) {
            setError("Audio transport is not connected")
        }
    }

    fun stopAudio() {
        if (!chatTransport.sendJson(audioStopMessage())) {
            _state.update { current -> current.copy(isRecording = false) }
        }
    }

    fun submitInk(strokes: List<TaburaInkStroke>) {
        if (strokes.isEmpty()) {
            return
        }
        val requestResponse = state.value.inkRequestsResponse
        if (!chatTransport.sendJson(inkCommitMessage(strokes, requestResponse))) {
            setError("Ink transport is not connected")
            return
        }
        _state.update { current ->
            current.copy(statusText = if (requestResponse) "Ink sent to Tabura" else "Ink captured")
        }
    }

    private suspend fun attachWorkspace(baseUrl: String, workspace: TaburaWorkspace) {
        activeWorkspace = workspace
        val history = loadHistory(baseUrl, workspace)
        _state.update { current ->
            current.copy(
                messages = history,
                selectedWorkspaceId = workspace.id,
            )
        }
        chatTransport.connect(baseUrl, workspace.chatSessionId)
        canvasTransport.connect(baseUrl, workspace.canvasSessionId)
        canvasTransport.loadSnapshot(baseUrl, workspace.canvasSessionId)?.let { artifact ->
            _state.update { current -> current.copy(canvas = artifact) }
        }
        _state.update { current ->
            current.copy(statusText = "Connected to ${workspace.name}")
        }
    }

    private suspend fun loginIfNeeded(baseUrl: String) {
        val setup = JSONObject(get(taburaApiUrl(baseUrl, "setup")))
        val authenticated = setup.optBoolean("authenticated")
        val hasPassword = setup.optBoolean("has_password")
        if (authenticated || !hasPassword) {
            return
        }
        val response = postJson(
            url = taburaApiUrl(baseUrl, "login"),
            body = loginRequest(state.value.password),
        )
        if (response.isBlank()) {
            return
        }
    }

    private suspend fun loadWorkspaces(baseUrl: String): TaburaWorkspaceListResponse {
        return parseWorkspaceListResponse(get(taburaApiUrl(baseUrl, "runtime/workspaces")))
    }

    private suspend fun loadHistory(baseUrl: String, workspace: TaburaWorkspace): List<TaburaRenderedMessage> {
        return parseChatHistory(get(taburaApiUrl(baseUrl, "chat/sessions/${workspace.chatSessionId}/history")))
    }

    private suspend fun get(url: String): String = withContext(Dispatchers.IO) {
        val request = Request.Builder().url(url).build()
        client.newCall(request).execute().use { response ->
            if (!response.isSuccessful) {
                error("HTTP ${response.code} for $url")
            }
            response.body?.string().orEmpty()
        }
    }

    private suspend fun postJson(url: String, body: String): String = withContext(Dispatchers.IO) {
        val request = Request.Builder()
            .url(url)
            .post(body.toRequestBody(jsonMediaType))
            .build()
        client.newCall(request).execute().use { response ->
            if (!response.isSuccessful) {
                error("HTTP ${response.code} for $url")
            }
            response.body?.string().orEmpty()
        }
    }

    private fun handleChatEvent(event: TaburaChatEventPayload) {
        when (event.type) {
            "render_chat", "assistant_output", "message_persisted" -> {
                val content = event.markdown.ifBlank { event.message.ifBlank { event.text } }
                if (content.isBlank()) {
                    return
                }
                _state.update { current ->
                    current.copy(
                        messages = current.messages + TaburaRenderedMessage(
                            id = event.turnId.ifBlank { "event-${System.currentTimeMillis()}" },
                            role = event.role.ifBlank { "assistant" },
                            text = content,
                            html = event.html,
                        )
                    )
                }
            }

            "stt_result" -> {
                if (event.text.isBlank()) {
                    return
                }
                _state.update { current ->
                    current.copy(
                        composerText = event.text,
                        statusText = "Transcription ready",
                    )
                }
            }

            "stt_empty" -> {
                _state.update { current ->
                    current.copy(statusText = event.reason.ifBlank { "No speech detected" })
                }
            }

            "stt_error", "error" -> setError(event.error.ifBlank { "Tabura server error" })
        }
    }

    private fun setError(message: String) {
        _state.update { current -> current.copy(lastError = message) }
    }
}
