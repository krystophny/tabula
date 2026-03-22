// swift-tools-version: 5.9
import PackageDescription

let package = Package(
    name: "TaburaFlowContract",
    products: [
        .library(
            name: "TaburaFlowContract",
            targets: ["TaburaFlowContract"]
        ),
    ],
    targets: [
        .target(
            name: "TaburaFlowContract"
        ),
        .testTarget(
            name: "TaburaFlowContractTests",
            dependencies: ["TaburaFlowContract"],
            resources: [.process("Resources")]
        ),
    ]
)
