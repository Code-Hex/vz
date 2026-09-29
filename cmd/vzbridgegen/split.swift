import Foundation
import SwiftParser
import SwiftSyntax

struct Request: Decodable {
    var sources: [String]
    var symbols: Set<String>
    var output: String
}

func exportedSymbol(_ function: FunctionDeclSyntax) -> String? {
    for element in function.attributes {
        guard let attribute = element.as(AttributeSyntax.self),
              attribute.attributeName.trimmedDescription == "c" else { continue }
        return attribute.arguments?.trimmedDescription
    }
    return nil
}

final class Exports: SyntaxVisitor {
    var symbols: Set<String> = []
    override func visit(_ node: FunctionDeclSyntax) -> SyntaxVisitorContinueKind {
        if let symbol = exportedSymbol(node) { symbols.insert(symbol) }
        return .skipChildren
    }
}

final class Projection: SyntaxRewriter {
    let symbol: String?
    init(symbol: String?) {
        self.symbol = symbol
        super.init(viewMode: .sourceAccurate)
    }
    override func visit(_ node: FunctionDeclSyntax) -> DeclSyntax { DeclSyntax(node) }
    override func visit(_ node: CodeBlockItemListSyntax) -> CodeBlockItemListSyntax {
        super.visit(node.filter { item in
            if let function = item.item.as(FunctionDeclSyntax.self) {
                return exportedSymbol(function) == symbol
            }
            return symbol == nil || item.item.is(ImportDeclSyntax.self) || item.item.is(IfConfigDeclSyntax.self)
        })
    }
}

let request = try JSONDecoder().decode(Request.self, from: Data(contentsOf: URL(fileURLWithPath: CommandLine.arguments[1])))
var outputs: [String] = []
var functions: [String: String] = [:]
for (index, path) in request.sources.enumerated() {
    let tree = Parser.parse(source: try String(contentsOfFile: path, encoding: .utf8))
    let exports = Exports(viewMode: .sourceAccurate)
    exports.walk(tree)
    let supportPath = "\(request.output)/Support_\(index).swift"
    try Projection(symbol: nil).rewrite(tree).description.write(toFile: supportPath, atomically: true, encoding: .utf8)
    outputs.append(supportPath)
    for symbol in exports.symbols.intersection(request.symbols) {
        functions[symbol, default: ""] += Projection(symbol: symbol).rewrite(tree).description + "\n"
    }
}
guard Set(functions.keys) == request.symbols else {
    fatalError("missing bridge symbols: \(request.symbols.subtracting(functions.keys).sorted())")
}
for (index, symbol) in functions.keys.sorted().enumerated() {
    let path = "\(request.output)/Function_\(index).swift"
    try functions[symbol]!.write(toFile: path, atomically: true, encoding: .utf8)
    outputs.append(path)
}
FileHandle.standardOutput.write(try JSONEncoder().encode(outputs))
