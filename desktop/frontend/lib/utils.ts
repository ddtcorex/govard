import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

/**
 * shadcn/ui's class composer. Every generated component imports this through the
 * "@/*" alias that components.json and vite.config.js both map.
 */
export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}
