const parts = (version: string) =>
  version
    .replace(/^v/, "")
    .split("-")[0]
    .split(".")
    .map((part) => Number.parseInt(part, 10) || 0);

/** Whether `candidate` is a newer release than `current`. Pre-releases never count. */
export const isNewer = (candidate: string, current: string) => {
  if (candidate.includes("-")) return false;
  const next = parts(candidate);
  const now = parts(current);
  for (let index = 0; index < 3; index++) {
    if ((next[index] ?? 0) !== (now[index] ?? 0))
      return (next[index] ?? 0) > (now[index] ?? 0);
  }
  // 1.0.0 is newer than 1.0.0-rc.1.
  return current.includes("-");
};
