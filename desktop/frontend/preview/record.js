// @ts-check

/**
 * Wraps a real bindings module so every call still reaches the real backend
 * unchanged, while additionally posting {module, service, method, args,
 * result|error} to the dev-only record middleware. Spec D2: record mode is the
 * same seam, wrapping the real implementation instead of faking it.
 * @param {{loadReal: () => Promise<any>, post: (entry: any) => Promise<void>, getModule: () => string}} opts
 */
export function createRecordingLoader({ loadReal, post, getModule }) {
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
                await post({ module: getModule(), service: serviceName, method: methodName, args, result });
                return result;
              } catch (err) {
                await post({
                  module: getModule(),
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
