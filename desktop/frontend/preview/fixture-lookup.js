// @ts-check

/**
 * @param {{service:string, method:string, args:any[], result?:any, error?:string}[]} fixtures
 * @param {string} service
 * @param {string} method
 * @param {any[]} args
 * @returns {{found:false} | {found:true, result?:any, error?:string}}
 */
export function resolveFixtureResponse(fixtures, service, method, args) {
  const sameRoute = fixtures.filter((f) => f.service === service && f.method === method);
  const argsKey = JSON.stringify(args);
  const exact = sameRoute.find((f) => JSON.stringify(f.args) === argsKey);
  const candidate = exact ?? sameRoute[0];
  if (!candidate) {
    return { found: false };
  }
  if ("error" in candidate) {
    return { found: true, error: candidate.error };
  }
  return { found: true, result: candidate.result };
}
