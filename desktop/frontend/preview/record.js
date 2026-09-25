// @ts-check

/**
 * Wraps a real bindings module so every call still reaches the real backend
 * unchanged, while additionally posting {service, method, args, result|error} to
 * the dev-only recorder. That entry is exactly the fixture format playback reads,
 * and it carries no file name: the Vite plugin owns the file and decides what to
 * call it (GOVARD_PREVIEW_RECORD_NAME, else the service). Naming it after
 * whatever view the app happened to be showing mixed unrelated routes into one
 * dump. Spec D2: record mode is the same seam, wrapping the real implementation
 * instead of faking it.
 * @param {{loadReal: () => Promise<any>, post: (entry: any) => Promise<void>}} opts
 */
export function createRecordingLoader({ loadReal, post }) {
  return async () => {
    const real = await loadReal();
    return new Proxy(real, {
      get(target, serviceName) {
        const service = target[serviceName];
        if (typeof serviceName !== "string" || !service) return service;
        return new Proxy(service, {
          get(serviceTarget, methodName) {
            const fn = serviceTarget[methodName];
            if (typeof methodName !== "string" || typeof fn !== "function") return fn;
            return async (...args) => {
              try {
                const result = await fn(...args);
                await post({ service: serviceName, method: methodName, args, result });
                return result;
              } catch (err) {
                await post({
                  service: serviceName,
                  method: methodName,
                  args,
                  error: err instanceof Error ? err.message : String(err),
                });
                throw err;
              }
            };
          },
        });
      },
    });
  };
}
