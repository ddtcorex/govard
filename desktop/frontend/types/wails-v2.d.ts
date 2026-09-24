// Shape of the globals Wails v2 injects. Only services/bridge.js and
// services/events.js may use them (enforced by a Go test).
export {};

declare global {
  interface Window {
    go?: {
      desktop?: {
        App?: Record<string, (...args: any[]) => Promise<any>>;
      };
    };
    runtime?: {
      EventsOn(name: string, handler: (data: any) => void): (() => void) | void;
    };
  }
}
