import { type ClassValue, clsx } from "clsx";
import { twMerge } from "tailwind-merge";

/** cn joins class names and lets the later Tailwind class win over an earlier one that conflicts. */
export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}
