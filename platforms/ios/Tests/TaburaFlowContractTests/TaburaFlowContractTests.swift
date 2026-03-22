import Foundation
import XCTest
@testable import TaburaFlowContract

final class TaburaFlowContractTests: XCTestCase {
    private struct FixtureLoadError: Error {}

    func testSharedFlowsExecuteOnIOSContract() throws {
        let bundle = try loadFixtureBundle()
        XCTAssertEqual(bundle.platform, "ios")
        XCTAssertFalse(bundle.flows.isEmpty)
        for flow in bundle.flows {
            try XCTContext.runActivity(named: flow.name) { _ in
                let runner = FlowRunner(platform: bundle.platform, selectors: bundle.selectors)
                try runner.run(flow: flow)
                print("ios PASS \(flow.name)")
            }
        }
    }

    private func loadFixtureBundle() throws -> FlowFixtureBundle {
        guard let url = Bundle.module.url(forResource: "flow-fixtures", withExtension: "json") else {
            XCTFail("missing flow fixture resource")
            throw FixtureLoadError()
        }
        let data = try Data(contentsOf: url)
        return try JSONDecoder().decode(FlowFixtureBundle.self, from: data)
    }
}
