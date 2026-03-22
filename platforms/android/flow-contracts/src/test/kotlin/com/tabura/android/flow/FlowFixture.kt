package com.tabura.android.flow

import org.json.JSONArray
import org.json.JSONObject

data class FlowFixtureBundle(
    val platform: String,
    val flows: List<FlowDefinition>,
    val selectors: Map<String, String>,
)

data class FlowDefinition(
    val name: String,
    val description: String,
    val file: String,
    val preconditions: FlowPreconditions?,
    val steps: List<FlowStep>,
)

data class FlowPreconditions(
    val tool: String?,
    val session: String?,
    val silent: Boolean?,
    val indicatorState: String?,
)

data class FlowStep(
    val action: String,
    val target: String?,
    val durationMs: Int?,
    val expect: FlowExpectations?,
    val platforms: List<String>?,
)

data class FlowExpectations(
    val activeTool: String?,
    val session: String?,
    val silent: Boolean?,
    val taburaCircle: String?,
    val dotInnerIcon: String?,
    val bodyClassContains: String?,
    val indicatorState: String?,
    val cursorClass: String?,
)

object FlowFixtureLoader {
    fun load(jsonText: String): FlowFixtureBundle {
        val root = JSONObject(jsonText)
        return FlowFixtureBundle(
            platform = root.getString("platform"),
            flows = root.getJSONArray("flows").toFlowDefinitions(),
            selectors = root.getJSONObject("selectors").toStringMap(),
        )
    }

    private fun JSONArray.toFlowDefinitions(): List<FlowDefinition> {
        return List(length()) { index ->
            getJSONObject(index).toFlowDefinition()
        }
    }

    private fun JSONObject.toFlowDefinition(): FlowDefinition {
        return FlowDefinition(
            name = getString("name"),
            description = getString("description"),
            file = getString("file"),
            preconditions = optJSONObject("preconditions")?.toPreconditions(),
            steps = getJSONArray("steps").toFlowSteps(),
        )
    }

    private fun JSONArray.toFlowSteps(): List<FlowStep> {
        return List(length()) { index ->
            getJSONObject(index).toFlowStep()
        }
    }

    private fun JSONObject.toFlowStep(): FlowStep {
        return FlowStep(
            action = getString("action"),
            target = optNullableString("target"),
            durationMs = if (has("duration_ms")) getInt("duration_ms") else null,
            expect = optJSONObject("expect")?.toExpectations(),
            platforms = optJSONArray("platforms")?.toStringList(),
        )
    }

    private fun JSONObject.toPreconditions(): FlowPreconditions {
        return FlowPreconditions(
            tool = optNullableString("tool"),
            session = optNullableString("session"),
            silent = if (has("silent")) getBoolean("silent") else null,
            indicatorState = optNullableString("indicator_state"),
        )
    }

    private fun JSONObject.toExpectations(): FlowExpectations {
        return FlowExpectations(
            activeTool = optNullableString("active_tool"),
            session = optNullableString("session"),
            silent = if (has("silent")) getBoolean("silent") else null,
            taburaCircle = optNullableString("tabura_circle"),
            dotInnerIcon = optNullableString("dot_inner_icon"),
            bodyClassContains = optNullableString("body_class_contains"),
            indicatorState = optNullableString("indicator_state"),
            cursorClass = optNullableString("cursor_class"),
        )
    }

    private fun JSONObject.toStringMap(): Map<String, String> {
        return keys().asSequence().associateWith { key -> getString(key) }
    }

    private fun JSONArray.toStringList(): List<String> {
        return List(length()) { index -> getString(index) }
    }

    private fun JSONObject.optNullableString(name: String): String? {
        if (!has(name) || isNull(name)) {
            return null
        }
        return getString(name)
    }
}
