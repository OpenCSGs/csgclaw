import { dependenciesInSource } from "./sourceDependencies";

test("finds type imports, re-exports and literal dynamic imports through aliases and relative paths", () => {
  const source = `import type { Value } from "@/models/value";
export { View } from "../../business/View";
const load = () => import("../../../pages/Example");`;
  expect(dependenciesInSource("components/ui/Control/Control.tsx", source)).toEqual([
    { source: "components/ui/Control/Control.tsx", target: "models/value", specifier: "@/models/value", line: 1 },
    {
      source: "components/ui/Control/Control.tsx",
      target: "components/business/View",
      specifier: "../../business/View",
      line: 2,
    },
    {
      source: "components/ui/Control/Control.tsx",
      target: "pages/Example",
      specifier: "../../../pages/Example",
      line: 3,
    },
  ]);
});

test("does not interpret comments, strings, package imports or runtime expressions as local dependencies", () => {
  const source = `import { useState } from "react";
// import { Client } from "@/api/client";
const example = 'import("@/api/client")';
const load = (name: string) => import(name);`;
  expect(dependenciesInSource("components/ui/Control.tsx", source)).toEqual([]);
});
