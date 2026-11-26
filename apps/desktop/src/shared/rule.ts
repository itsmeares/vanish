import { z } from "zod";

import type { Kind } from "./platforms";

/** One thing Vanish found on an account. */
export type Item = {
  id: string;
  kind: Kind;
  community: string;
  /** The post title, or the title of the thread a comment is in. */
  title: string;
  body: string;
  url: string;
  score: number;
  /** When it was posted, in milliseconds. */
  createdAt: number;
};

const DAY = 86_400_000;

export const AGE_PRESETS = [
  { days: 30, label: "1 month" },
  { days: 91, label: "3 months" },
  { days: 182, label: "6 months" },
  { days: 365, label: "1 year" },
  { days: 730, label: "2 years" },
  { days: 1825, label: "5 years" },
] as const;

const keepSchema = z.discriminatedUnion("type", [
  z.object({ type: z.literal("community"), value: z.string().min(1).max(100) }),
  z.object({ type: z.literal("score"), min: z.number().int() }),
  z.object({ type: z.literal("text"), value: z.string().min(1).max(200) }),
]);
export type KeepRule = z.infer<typeof keepSchema>;

export const ruleSchema = z.object({
  /** Empty means every community. */
  communities: z.array(z.string().min(1).max(100)).max(5000),
  age: z.discriminatedUnion("type", [
    z.object({ type: z.literal("any") }),
    // Relative, so a saved rule still means "older than a year" next month.
    z.object({ type: z.literal("older"), days: z.number().int().positive() }),
    z.object({
      type: z.literal("between"),
      from: z.number().int(),
      to: z.number().int(),
    }),
  ]),
  /** Only items with fewer points than this. */
  maxScore: z.number().int().nullable(),
  /** Only items containing this text. Empty means any text. */
  text: z.string().max(200),
  keep: z.array(keepSchema).max(100),
  /** Items picked one by one to keep. */
  keepIds: z.array(z.string().min(1).max(200)).max(200_000),
});
export type Rule = z.infer<typeof ruleSchema>;

export const DEFAULT_RULE: Rule = {
  communities: [],
  age: { type: "any" },
  maxScore: null,
  text: "",
  keep: [],
  keepIds: [],
};

/** Out of the sentence, kept by a rule or by hand, or on the delete list. */
export type Verdict =
  | { type: "out" }
  | { type: "kept"; reason: string }
  | { type: "goes" };

const contains = (item: Item, text: string) => {
  const needle = text.toLocaleLowerCase();
  return (
    item.body.toLocaleLowerCase().includes(needle) ||
    item.title.toLocaleLowerCase().includes(needle)
  );
};

const inAge = (item: Item, age: Rule["age"], now: number) => {
  if (age.type === "any") return true;
  if (age.type === "older") return item.createdAt < now - age.days * DAY;
  return item.createdAt >= age.from && item.createdAt < age.to;
};

export const keepLabel = (keep: KeepRule, prefix: string) => {
  if (keep.type === "community") return `${prefix}${keep.value}`;
  if (keep.type === "score")
    return `over ${keep.min.toLocaleString("en")} points`;
  return `containing "${keep.value}"`;
};

/** Builds the judge once per rule, so a 100,000 item list stays fast. */
export const judge = (rule: Rule, now: number, prefix: string) => {
  const communities = new Set(
    rule.communities.map((name) => name.toLowerCase()),
  );
  const keepIds = new Set(rule.keepIds);
  return (item: Item): Verdict => {
    if (communities.size > 0 && !communities.has(item.community.toLowerCase()))
      return { type: "out" };
    if (!inAge(item, rule.age, now)) return { type: "out" };
    if (rule.maxScore !== null && item.score >= rule.maxScore)
      return { type: "out" };
    if (rule.text && !contains(item, rule.text)) return { type: "out" };
    for (const keep of rule.keep) {
      const kept =
        keep.type === "community"
          ? item.community.toLowerCase() === keep.value.toLowerCase()
          : keep.type === "score"
            ? item.score > keep.min
            : contains(item, keep.value);
      if (kept) return { type: "kept", reason: keepLabel(keep, prefix) };
    }
    if (keepIds.has(item.id)) return { type: "kept", reason: "kept by you" };
    return { type: "goes" };
  };
};

export const ageLabel = (age: Rule["age"]) => {
  if (age.type === "any") return "from any time";
  if (age.type === "older") {
    const preset = AGE_PRESETS.find((option) => option.days === age.days);
    return `older than ${preset ? preset.label : `${age.days} days`}`;
  }
  const month = (time: number) =>
    new Date(time).toLocaleDateString("en", {
      month: "short",
      year: "numeric",
    });
  // `to` is the first moment after the range, so name the month before it.
  return `from ${month(age.from)} to ${month(age.to - 1)}`;
};
