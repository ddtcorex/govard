/**
 * A visibility-aware interval for background polling (metrics, log polls).
 * Ticks only while the document is visible; returning to visible fires once
 * immediately, then restarts the cadence. Hiding clears the timer without
 * firing. Plain factory (not a hook) so node:test can drive it with stubbed
 * document and timers; islands wrap it in a small useEffect.
 */
export function createVisiblePoll({
  intervalMs,
  onTick,
  doc,
  setTimer = setInterval,
  clearTimer = clearInterval,
}) {
  let timer = null;

  const isHidden = () => doc.visibilityState === "hidden";

  const stop = () => {
    if (timer !== null) {
      clearTimer(timer);
      timer = null;
    }
  };

  const start = () => {
    stop();
    if (!isHidden()) {
      timer = setTimer(() => onTick(), intervalMs);
    }
  };

  const handleVisibility = () => {
    if (isHidden()) {
      stop();
    } else {
      onTick();
      start();
    }
  };

  doc.addEventListener("visibilitychange", handleVisibility);
  start();

  return {
    stop,
    dispose: () => {
      doc.removeEventListener("visibilitychange", handleVisibility);
      stop();
    },
  };
}
