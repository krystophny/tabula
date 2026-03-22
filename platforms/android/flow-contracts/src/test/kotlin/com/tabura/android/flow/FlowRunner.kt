package com.tabura.android.flow

data class FlowSnapshot(
    val activeTool: String,
    val session: String,
    val silent: Boolean,
    val taburaCircle: String,
    val dotInnerIcon: String,
    val indicatorState: String,
    val bodyClass: String,
    val cursorClass: String,
)

class FlowRunner(
    private val platform: String,
    private val selectors: Map<String, String>,
) {
    private data class State(
        var tool: String = "pointer",
        var session: String = "none",
        var silent: Boolean = false,
        var circleExpanded: Boolean = false,
        var indicatorOverride: String = "",
    )

    private val state = State()

    fun run(flow: FlowDefinition) {
        reset(flow.preconditions)
        for (step in flow.steps) {
            if (shouldSkip(step)) {
                continue
            }
            runStep(step)
        }
    }

    private fun reset(preconditions: FlowPreconditions?) {
        state.tool = normalizeTool(preconditions?.tool ?: "pointer")
        state.session = normalizeSession(preconditions?.session ?: "none")
        state.silent = preconditions?.silent ?: false
        state.circleExpanded = false
        state.indicatorOverride = normalizeIndicator(preconditions?.indicatorState ?: "")
    }

    private fun shouldSkip(step: FlowStep): Boolean {
        val platforms = step.platforms ?: return false
        return platform !in platforms
    }

    private fun runStep(step: FlowStep) {
        when (step.action) {
            "tap" -> tap(requireTarget(step))
            "tap_outside" -> state.circleExpanded = false
            "verify" -> step.target?.let(::resolveSelector)
            "wait" -> step.durationMs ?: 0
            else -> error("unsupported action ${step.action}")
        }
        assertExpectations(step.expect)
    }

    private fun tap(target: String) {
        resolveSelector(target)
        when (target) {
            "tabura_circle_dot" -> state.circleExpanded = !state.circleExpanded
            "tabura_circle_segment_pointer" -> state.tool = "pointer"
            "tabura_circle_segment_highlight" -> state.tool = "highlight"
            "tabura_circle_segment_ink" -> state.tool = "ink"
            "tabura_circle_segment_text_note" -> state.tool = "text_note"
            "tabura_circle_segment_prompt" -> state.tool = "prompt"
            "tabura_circle_segment_dialogue" -> toggleSession("dialogue")
            "tabura_circle_segment_meeting" -> toggleSession("meeting")
            "tabura_circle_segment_silent" -> state.silent = !state.silent
            "indicator_border" -> {
                state.session = "none"
                state.indicatorOverride = ""
            }
            else -> error("unsupported tap target $target")
        }
    }

    private fun toggleSession(next: String) {
        state.session = if (state.session == next) "none" else next
        state.indicatorOverride = ""
    }

    private fun requireTarget(step: FlowStep): String {
        return step.target?.takeIf { it.isNotBlank() }
            ?: error("missing target for action ${step.action}")
    }

    private fun resolveSelector(target: String): String {
        return selectors[target]?.takeIf { it.isNotBlank() }
            ?: error("missing selector for target $target")
    }

    private fun assertExpectations(expect: FlowExpectations?) {
        if (expect == null) {
            return
        }
        val snapshot = snapshot()
        expect.activeTool?.let { check(snapshot.activeTool == it) { "expected active_tool $it, got ${snapshot.activeTool}" } }
        expect.session?.let { check(snapshot.session == it) { "expected session $it, got ${snapshot.session}" } }
        expect.silent?.let { check(snapshot.silent == it) { "expected silent $it, got ${snapshot.silent}" } }
        expect.taburaCircle?.let { check(snapshot.taburaCircle == it) { "expected tabura_circle $it, got ${snapshot.taburaCircle}" } }
        expect.dotInnerIcon?.let { check(snapshot.dotInnerIcon == it) { "expected dot_inner_icon $it, got ${snapshot.dotInnerIcon}" } }
        expect.bodyClassContains?.let {
            check(snapshot.bodyClass.contains(it)) {
                "expected body_class to contain $it, got ${snapshot.bodyClass}"
            }
        }
        expect.indicatorState?.let {
            check(snapshot.indicatorState == it) { "expected indicator_state $it, got ${snapshot.indicatorState}" }
        }
        expect.cursorClass?.let { check(snapshot.cursorClass == it) { "expected cursor_class $it, got ${snapshot.cursorClass}" } }
    }

    private fun snapshot(): FlowSnapshot {
        val indicatorState = currentIndicatorState()
        val bodyClass = listOf(
            "tool-${state.tool}",
            "session-${state.session}",
            "indicator-$indicatorState",
            if (state.silent) "silent-on" else "silent-off",
            if (state.circleExpanded) "circle-expanded" else "circle-collapsed",
        ).joinToString(" ")
        return FlowSnapshot(
            activeTool = state.tool,
            session = state.session,
            silent = state.silent,
            taburaCircle = if (state.circleExpanded) "expanded" else "collapsed",
            dotInnerIcon = toolIconId(state.tool),
            indicatorState = indicatorState,
            bodyClass = bodyClass,
            cursorClass = "tool-${state.tool}",
        )
    }

    private fun currentIndicatorState(): String {
        if (state.indicatorOverride.isNotBlank()) {
            return state.indicatorOverride
        }
        return when (state.session) {
            "dialogue" -> "listening"
            "meeting" -> "paused"
            else -> "idle"
        }
    }

    private fun normalizeTool(value: String): String {
        return when (value) {
            "pointer", "highlight", "ink", "text_note", "prompt" -> value
            else -> "pointer"
        }
    }

    private fun normalizeSession(value: String): String {
        return when (value) {
            "none", "dialogue", "meeting" -> value
            else -> "none"
        }
    }

    private fun normalizeIndicator(value: String): String {
        return when (value) {
            "idle", "listening", "paused", "recording", "working" -> value
            else -> ""
        }
    }

    private fun toolIconId(tool: String): String {
        return when (tool) {
            "highlight" -> "marker"
            "ink" -> "pen_nib"
            "text_note" -> "sticky_note"
            "prompt" -> "mic"
            else -> "arrow"
        }
    }
}
