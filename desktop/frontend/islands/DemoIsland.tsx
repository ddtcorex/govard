import { useState } from "react";
import { useStore } from "../state/useStore.js";
import { Button } from "../components/ui/button";

/**
 * Throwaway proof of the mount/unmount and D5 "owns its subtree" contract, plus
 * the D8 token adapter: the only interactive element is shadcn's generated
 * Button, so a CDP assertion on its computed background proves the adapter maps
 * shadcn's variable names onto Govard's real tokens.
 *
 * Never imported from main.js or index.html; mounted only from
 * preview/bootstrap.js into preview.html's own container, so it never ships
 * (dist never contains preview.html, per Task 1's guard test).
 */
export function DemoIsland() {
  const [count, setCount] = useState(0);
  const state = useStore();
  return (
    <div>
      <p>Count: {count}</p>
      <p>sidebarMode: {state.sidebarMode}</p>
      <Button data-testid="demo-island-increment" onClick={() => setCount((c) => c + 1)}>
        Increment
      </Button>
    </div>
  );
}
