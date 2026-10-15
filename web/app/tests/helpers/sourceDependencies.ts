import { readdirSync, readFileSync } from "node:fs";
import { posix, relative, resolve, sep } from "node:path";
import ts from "typescript";

export type SourceDependency = {
  source: string;
  target: string;
  specifier: string;
  line: number;
};

export function dependenciesInSource(file: string, text: string): SourceDependency[] {
  const name = file.replaceAll("\\", "/");
  const source = ts.createSourceFile(name, text, ts.ScriptTarget.Latest, true);
  const dependencies: SourceDependency[] = [];

  function add(node: ts.Node, specifier: string) {
    const target = specifier.startsWith("@/")
      ? posix.normalize(specifier.slice(2))
      : specifier.startsWith(".")
        ? posix.normalize(posix.join(posix.dirname(name), specifier))
        : null;
    if (!target) return;
    dependencies.push({
      source: name,
      target,
      specifier,
      line: source.getLineAndCharacterOfPosition(node.getStart(source)).line + 1,
    });
  }

  function visit(node: ts.Node) {
    if (
      (ts.isImportDeclaration(node) || ts.isExportDeclaration(node)) &&
      node.moduleSpecifier &&
      ts.isStringLiteral(node.moduleSpecifier)
    ) {
      add(node, node.moduleSpecifier.text);
    } else if (
      ts.isCallExpression(node) &&
      node.expression.kind === ts.SyntaxKind.ImportKeyword &&
      node.arguments.length &&
      ts.isStringLiteral(node.arguments[0])
    ) {
      add(node, node.arguments[0].text);
    }
    ts.forEachChild(node, visit);
  }

  visit(source);
  return dependencies;
}

export function readSourceDependencies(sourceRoot: string): SourceDependency[] {
  function walk(directory: string): SourceDependency[] {
    return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
      const file = resolve(directory, entry.name);
      if (entry.isDirectory()) return walk(file);
      if (!/\.(ts|tsx)$/.test(entry.name) || /\.(?:test|spec)\.[jt]sx?$|\.d\.ts$/.test(entry.name)) return [];
      return dependenciesInSource(relative(sourceRoot, file).split(sep).join("/"), readFileSync(file, "utf8"));
    });
  }

  return walk(sourceRoot);
}
