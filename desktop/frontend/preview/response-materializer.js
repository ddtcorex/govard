// @ts-check
import { loadGeneratedModules } from "../services/bridge.js";
import { ROUTE_DEFAULTS } from "./route-defaults.generated.js";

/**
 * Turns a resolved fixture (or nothing) into the value a real bridge/binding
 * call would produce, always going through the real generated model class for a
 * "model" route (spec D4) - never returning raw JSON directly. The model classes
 * come from bridge.js's loader for the REAL generated module, because the
 * swappable one hands out the fake module under the loader seam.
 * @param {string} service
 * @param {string} method
 * @param {any} [rawResult]
 * @returns {Promise<any>}
 */
export async function materializeResponse(service, method, rawResult) {
  const descriptor = ROUTE_DEFAULTS[`${service}.${method}`];
  if (!descriptor) {
    throw new Error(
      `preview: no route default for ${service}.${method}; regenerate route-defaults.generated.js`,
    );
  }
  if (descriptor.kind === "void") {
    return undefined;
  }
  if (descriptor.kind === "slice") {
    return rawResult ?? [];
  }
  if (descriptor.kind === "value") {
    return rawResult ?? descriptor.value;
  }
  const modules = await loadGeneratedModules();
  const model = modules[descriptor.model];
  if (!model || typeof model.createFrom !== "function") {
    throw new Error(
      `preview: the generated bindings export no model class named ${descriptor.model}`,
    );
  }
  return model.createFrom(rawResult ?? {});
}
