import { toastManager } from "@vanish/ui/components/toast";

// Electron wraps errors from the main process as
// "Error invoking remote method 'name': Error: message".
const message = (error: unknown) =>
  String(error instanceof Error ? error.message : error).replace(
    /^Error invoking remote method '[^']+': (?:\w*Error: )?/,
    "",
  );

/** Runs a main-process call and shows its error, if any, as a toast. */
export const attempt = async <T>(
  work: () => Promise<T>,
): Promise<T | undefined> => {
  try {
    return await work();
  } catch (error) {
    toastManager.add({ type: "error", title: message(error) });
    return undefined;
  }
};

export const notify = (title: string, description?: string) =>
  toastManager.add({ type: "success", title, description });
