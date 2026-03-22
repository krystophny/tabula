import Foundation
import XCTest
@testable import TaburaIOSModels

final class TaburaModelContractTests: XCTestCase {
    func testChatEventDecodesDialogueActionFields() throws {
        let data = Data(
            """
            {
              "type": "action",
              "state": "listening",
              "workspace_path": "/tmp/workspace",
              "action": { "type": "toggle_live_dialogue" }
            }
            """.utf8
        )

        let payload = try JSONDecoder().decode(TaburaChatEventPayload.self, from: data)
        XCTAssertEqual(payload.type, "action")
        XCTAssertEqual(payload.state, "listening")
        XCTAssertEqual(payload.workspacePath, "/tmp/workspace")
        XCTAssertEqual(payload.actionType, "toggle_live_dialogue")
    }

    func testTransportHelpersPreserveThinClientPaths() {
        let baseURL = URL(string: "http://tabura.local:8420")!
        XCTAssertEqual(
            taburaAPIURL(baseURL: baseURL, path: "chat/sessions/session-1/history").absoluteString,
            "http://tabura.local:8420/api/chat/sessions/session-1/history"
        )
        XCTAssertEqual(
            taburaWSURL(baseURL: baseURL, path: "chat/sessions/session-1")?.absoluteString,
            "ws://tabura.local:8420/ws/chat/sessions/session-1"
        )
    }

    func testCanvasFallbackEscapesText() throws {
        let data = Data(
            """
            {
              "kind": "text",
              "title": "Canvas",
              "text": "<note>&raw"
            }
            """.utf8
        )
        let payload = try JSONDecoder().decode(TaburaCanvasEventPayload.self, from: data)
        XCTAssertEqual(
            taburaCanvasHTML(from: payload),
            "<pre style=\"white-space: pre-wrap; font: -apple-system-body; margin: 24px;\">&lt;note&gt;&amp;raw</pre>"
        )
    }

    func testRequestEncodingMatchesThinClientWireFormat() throws {
        let patchData = try JSONEncoder().encode(
            TaburaCompanionConfigPatch(companionEnabled: true, idleSurface: "black")
        )
        let patchObject = try XCTUnwrap(
            JSONSerialization.jsonObject(with: patchData) as? [String: Any]
        )
        XCTAssertEqual(patchObject["companion_enabled"] as? Bool, true)
        XCTAssertEqual(patchObject["idle_surface"] as? String, "black")

        let inkData = try JSONEncoder().encode(
            TaburaInkCommitMessage(
                type: "ink_stroke",
                artifactKind: "text",
                requestResponse: false,
                outputMode: "voice",
                totalStrokes: 1,
                strokes: [
                    TaburaInkStroke(
                        pointerType: "stylus",
                        width: 2.5,
                        points: [
                            TaburaInkPoint(
                                x: 1,
                                y: 2,
                                pressure: 0.5,
                                tiltX: 3,
                                tiltY: 4,
                                roll: 5,
                                timestampMS: 6
                            ),
                        ]
                    ),
                ]
            )
        )
        let inkObject = try XCTUnwrap(
            JSONSerialization.jsonObject(with: inkData) as? [String: Any]
        )
        XCTAssertEqual(inkObject["type"] as? String, "ink_stroke")
        XCTAssertEqual(inkObject["request_response"] as? Bool, false)
        XCTAssertEqual(inkObject["total_strokes"] as? Int, 1)
    }
}
