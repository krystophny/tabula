import Foundation

public struct FlowFixtureBundle: Decodable {
    public let platform: String
    public let flows: [FlowDefinition]
    public let selectors: [String: String]
}

public struct FlowDefinition: Decodable {
    public let name: String
    public let description: String
    public let file: String
    public let preconditions: FlowPreconditions?
    public let steps: [FlowStep]
}

public struct FlowPreconditions: Decodable {
    public let tool: String?
    public let session: String?
    public let silent: Bool?
    public let indicatorState: String?

    enum CodingKeys: String, CodingKey {
        case tool
        case session
        case silent
        case indicatorState = "indicator_state"
    }
}

public struct FlowStep: Decodable {
    public let action: String
    public let target: String?
    public let durationMS: Int?
    public let expect: FlowExpectations?
    public let platforms: [String]?

    enum CodingKeys: String, CodingKey {
        case action
        case target
        case durationMS = "duration_ms"
        case expect
        case platforms
    }
}

public struct FlowExpectations: Decodable {
    public let activeTool: String?
    public let session: String?
    public let silent: Bool?
    public let taburaCircle: String?
    public let dotInnerIcon: String?
    public let bodyClassContains: String?
    public let indicatorState: String?
    public let cursorClass: String?

    enum CodingKeys: String, CodingKey {
        case activeTool = "active_tool"
        case session
        case silent
        case taburaCircle = "tabura_circle"
        case dotInnerIcon = "dot_inner_icon"
        case bodyClassContains = "body_class_contains"
        case indicatorState = "indicator_state"
        case cursorClass = "cursor_class"
    }
}
