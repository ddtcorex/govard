import test from "node:test";
import assert from "node:assert/strict";
import {
  parseServiceReturnTypes,
  classifyReturnType,
  parseServiceMethodIds,
} from "../../../desktop/frontend/scripts/route-defaults-parser.mjs";

const FIXTURE_SOURCE = `
/**
 * @returns {$CancellablePromise<string>}
 */
export function GetMailpitURL() {
    return $Call.ByID(1098421658);
}

/**
 * @returns {$CancellablePromise<$models.DesktopSettings>}
 */
export function GetSettings() {
    return $Call.ByID(2638684590);
}

/**
 * @returns {$CancellablePromise<void>}
 */
export function Quit() {
    return $Call.ByID(1457209326);
}
`;

test("parseServiceReturnTypes extracts every exported function's return type", () => {
  const parsed = parseServiceReturnTypes(FIXTURE_SOURCE);
  assert.deepEqual(parsed, [
    { name: "GetMailpitURL", type: "string" },
    { name: "GetSettings", type: "$models.DesktopSettings" },
    { name: "Quit", type: "void" },
  ]);
});

test("classifyReturnType maps every kind D4 describes", () => {
  assert.deepEqual(classifyReturnType("void"), { kind: "void" });
  assert.deepEqual(classifyReturnType("string"), { kind: "value", value: "" });
  assert.deepEqual(classifyReturnType("number"), { kind: "value", value: 0 });
  assert.deepEqual(classifyReturnType("boolean"), { kind: "value", value: false });
  assert.deepEqual(classifyReturnType("string[]"), { kind: "slice" });
  assert.deepEqual(classifyReturnType("$models.Dashboard"), {
    kind: "model",
    model: "Dashboard",
  });
});

test("classifyReturnType falls back to a null value for an unrecognized type, not a throw", () => {
  assert.deepEqual(classifyReturnType("Record<string, any>"), {
    kind: "value",
    value: null,
  });
});

// A regex that demands a closing paren right after the digits, such as
// /ByID\((\d+)\)/, matches only zero-argument methods: the generated tree has 23
// of those and 37 that forward an argument, and the 37 would drop out of the id
// map silently. This fixture pins the argument-forwarding case.
test("parseServiceMethodIds finds the id of a method that forwards an argument", () => {
  const parsed = parseServiceMethodIds(FIXTURE_SOURCE);
  assert.deepEqual(parsed, [
    { name: "GetMailpitURL", id: "1098421658" },
    { name: "GetSettings", id: "2638684590" },
    { name: "Quit", id: "1457209326" },
  ]);
});

test("parseServiceMethodIds keeps methods whose arguments follow the id", () => {
  const source = `
/**
 * @returns {$CancellablePromise<string>}
 */
export function DeleteProject(projectQuery) {
    return $Call.ByID(1866001717, projectQuery);
}

/**
 * @returns {$CancellablePromise<void>}
 */
export function Quit() {
    return $Call.ByID(1457209326);
}
`;
  assert.deepEqual(parseServiceMethodIds(source), [
    { name: "DeleteProject", id: "1866001717" },
    { name: "Quit", id: "1457209326" },
  ]);
});
