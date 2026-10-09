// @ts-check
import { Component, createElement } from "react";

/**
 * @typedef {object} ErrorBoundaryProps
 * @property {import("react").ReactNode} children
 * @property {() => void} onReset
 */

/**
 * Catches render throws inside one island and replaces that island's
 * subtree with a notice instead of letting React unmount it silently.
 * Mounted by mountIsland around every island; recovery re-renders the
 * same element, so the island boots fresh.
 *
 * Plain JS (not tsx) on purpose: mount.js is imported by node:test unit
 * tests, and Node cannot load .tsx. JSX is written as createElement calls.
 *
 * @extends {Component<ErrorBoundaryProps>}
 */
export class ErrorBoundary extends Component {
  /** @type {{ failed: boolean }} */
  state = { failed: false };

  /**
   * @returns {{ failed: boolean }}
   */
  static getDerivedStateFromError() {
    return { failed: true };
  }

  retry = () => {
    this.setState({ failed: false });
    this.props.onReset();
  };

  render() {
    if (!this.state.failed) {
      return this.props.children;
    }
    return createElement(
      "div",
      { role: "alert", className: "flex flex-col items-start gap-3 p-6 text-sm" },
      createElement("p", { className: "font-semibold" }, "Something went wrong in this panel."),
      createElement(
        "button",
        {
          type: "button",
          "data-testid": "island-error-retry",
          className: "px-3 py-2 rounded-lg border",
          onClick: this.retry,
        },
        "Try again",
      ),
    );
  }
}
