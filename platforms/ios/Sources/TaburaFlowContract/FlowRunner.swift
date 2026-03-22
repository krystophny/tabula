import Foundation

public struct FlowSnapshot {
    let activeTool: String
    let session: String
    let silent: Bool
    let taburaCircle: String
    let dotInnerIcon: String
    let indicatorState: String
    let bodyClass: String
    let cursorClass: String
}

public enum FlowRunnerError: Error, CustomStringConvertible {
    case missingSelector(String)
    case unsupportedAction(String)
    case missingTarget(String)
    case expectationFailed(String)

    public var description: String {
        switch self {
        case .missingSelector(let target):
            return "missing selector for target \(target)"
        case .unsupportedAction(let action):
            return "unsupported action \(action)"
        case .missingTarget(let action):
            return "missing target for action \(action)"
        case .expectationFailed(let detail):
            return detail
        }
    }
}

public final class FlowRunner {
    private struct State {
        var tool = "pointer"
        var session = "none"
        var silent = false
        var circleExpanded = false
        var indicatorOverride = ""
    }

    private let platform: String
    private let selectors: [String: String]
    private var state = State()

    public init(platform: String, selectors: [String: String]) {
        self.platform = platform
        self.selectors = selectors
    }

    public func run(flow: FlowDefinition) throws {
        reset(using: flow.preconditions)
        for step in flow.steps {
            if shouldSkip(step: step) {
                continue
            }
            try run(step: step)
        }
    }

    private func reset(using preconditions: FlowPreconditions?) {
        state.tool = normalizeTool(preconditions?.tool ?? "pointer")
        state.session = normalizeSession(preconditions?.session ?? "none")
        state.silent = preconditions?.silent ?? false
        state.circleExpanded = false
        state.indicatorOverride = normalizeIndicator(preconditions?.indicatorState ?? "")
    }

    private func shouldSkip(step: FlowStep) -> Bool {
        guard let platforms = step.platforms else {
            return false
        }
        return platforms.contains(platform) == false
    }

    private func run(step: FlowStep) throws {
        switch step.action {
        case "tap":
            try tap(target: requireTarget(step))
        case "tap_outside":
            state.circleExpanded = false
        case "verify":
            if let target = step.target {
                _ = try resolveSelector(for: target)
            }
        case "wait":
            _ = step.durationMS ?? 0
        default:
            throw FlowRunnerError.unsupportedAction(step.action)
        }
        try assertExpectations(step.expect)
    }

    private func tap(target: String) throws {
        _ = try resolveSelector(for: target)
        switch target {
        case "tabura_circle_dot":
            state.circleExpanded.toggle()
        case "tabura_circle_segment_pointer":
            state.tool = "pointer"
        case "tabura_circle_segment_highlight":
            state.tool = "highlight"
        case "tabura_circle_segment_ink":
            state.tool = "ink"
        case "tabura_circle_segment_text_note":
            state.tool = "text_note"
        case "tabura_circle_segment_prompt":
            state.tool = "prompt"
        case "tabura_circle_segment_dialogue":
            toggleSession("dialogue")
        case "tabura_circle_segment_meeting":
            toggleSession("meeting")
        case "tabura_circle_segment_silent":
            state.silent.toggle()
        case "indicator_border":
            state.session = "none"
            state.indicatorOverride = ""
        default:
            throw FlowRunnerError.unsupportedAction("tap:\(target)")
        }
    }

    private func toggleSession(_ next: String) {
        state.session = state.session == next ? "none" : next
        state.indicatorOverride = ""
    }

    private func resolveSelector(for target: String) throws -> String {
        guard let selector = selectors[target], selector.isEmpty == false else {
            throw FlowRunnerError.missingSelector(target)
        }
        return selector
    }

    private func requireTarget(_ step: FlowStep) throws -> String {
        guard let target = step.target, target.isEmpty == false else {
            throw FlowRunnerError.missingTarget(step.action)
        }
        return target
    }

    private func assertExpectations(_ expectations: FlowExpectations?) throws {
        guard let expectations else {
            return
        }
        let snapshot = snapshot()
        if let activeTool = expectations.activeTool, snapshot.activeTool != activeTool {
            throw FlowRunnerError.expectationFailed("expected active_tool \(activeTool), got \(snapshot.activeTool)")
        }
        if let session = expectations.session, snapshot.session != session {
            throw FlowRunnerError.expectationFailed("expected session \(session), got \(snapshot.session)")
        }
        if let silent = expectations.silent, snapshot.silent != silent {
            throw FlowRunnerError.expectationFailed("expected silent \(silent), got \(snapshot.silent)")
        }
        if let taburaCircle = expectations.taburaCircle, snapshot.taburaCircle != taburaCircle {
            throw FlowRunnerError.expectationFailed("expected tabura_circle \(taburaCircle), got \(snapshot.taburaCircle)")
        }
        if let dotInnerIcon = expectations.dotInnerIcon, snapshot.dotInnerIcon != dotInnerIcon {
            throw FlowRunnerError.expectationFailed("expected dot_inner_icon \(dotInnerIcon), got \(snapshot.dotInnerIcon)")
        }
        if let bodyClassContains = expectations.bodyClassContains, snapshot.bodyClass.contains(bodyClassContains) == false {
            throw FlowRunnerError.expectationFailed("expected body_class to contain \(bodyClassContains), got \(snapshot.bodyClass)")
        }
        if let indicatorState = expectations.indicatorState, snapshot.indicatorState != indicatorState {
            throw FlowRunnerError.expectationFailed("expected indicator_state \(indicatorState), got \(snapshot.indicatorState)")
        }
        if let cursorClass = expectations.cursorClass, snapshot.cursorClass != cursorClass {
            throw FlowRunnerError.expectationFailed("expected cursor_class \(cursorClass), got \(snapshot.cursorClass)")
        }
    }

    private func snapshot() -> FlowSnapshot {
        let indicatorState = currentIndicatorState()
        let bodyClass = [
            "tool-\(state.tool)",
            "session-\(state.session)",
            "indicator-\(indicatorState)",
            state.silent ? "silent-on" : "silent-off",
            state.circleExpanded ? "circle-expanded" : "circle-collapsed",
        ].joined(separator: " ")
        return FlowSnapshot(
            activeTool: state.tool,
            session: state.session,
            silent: state.silent,
            taburaCircle: state.circleExpanded ? "expanded" : "collapsed",
            dotInnerIcon: toolIconID(for: state.tool),
            indicatorState: indicatorState,
            bodyClass: bodyClass,
            cursorClass: "tool-\(state.tool)"
        )
    }

    private func currentIndicatorState() -> String {
        if state.indicatorOverride.isEmpty == false {
            return state.indicatorOverride
        }
        switch state.session {
        case "dialogue":
            return "listening"
        case "meeting":
            return "paused"
        default:
            return "idle"
        }
    }

    private func normalizeTool(_ value: String) -> String {
        switch value {
        case "pointer", "highlight", "ink", "text_note", "prompt":
            return value
        default:
            return "pointer"
        }
    }

    private func normalizeSession(_ value: String) -> String {
        switch value {
        case "none", "dialogue", "meeting":
            return value
        default:
            return "none"
        }
    }

    private func normalizeIndicator(_ value: String) -> String {
        switch value {
        case "idle", "listening", "paused", "recording", "working":
            return value
        default:
            return ""
        }
    }

    private func toolIconID(for tool: String) -> String {
        switch tool {
        case "highlight":
            return "marker"
        case "ink":
            return "pen_nib"
        case "text_note":
            return "sticky_note"
        case "prompt":
            return "mic"
        default:
            return "arrow"
        }
    }
}
