export const count = (value: number) => value.toLocaleString("en");

export const compact = (value: number) =>
  Math.abs(value) >= 1000
    ? `${(value / 1000).toFixed(value >= 10_000 ? 0 : 1).replace(/\.0$/, "")}k`
    : String(value);

export const points = (value: number) =>
  `${compact(value)} ${Math.abs(value) === 1 ? "point" : "points"}`;

export const month = (time: number) =>
  new Date(time).toLocaleDateString("en", { month: "short", year: "numeric" });

export const day = (time: number) =>
  new Date(time).toLocaleDateString("en", {
    day: "numeric",
    month: "short",
    year: "numeric",
  });

export const ago = (time: number, now = Date.now()) => {
  const minutes = Math.round((now - time) / 60_000);
  if (minutes < 1) return "just now";
  if (minutes < 60) return `${minutes} min ago`;
  const hours = Math.round(minutes / 60);
  if (hours < 24) return `${hours} h ago`;
  return day(time);
};

export const plural = (value: number, one: string, many: string) =>
  `${count(value)} ${value === 1 ? one : many}`;
