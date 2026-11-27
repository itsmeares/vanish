import { describe, expect, it } from "vite-plus/test";

import {
  ageLabel,
  DEFAULT_RULE,
  judge,
  ruleSchema,
  type Item,
  type Rule,
} from "./rule";

const NOW = Date.UTC(2026, 10, 26);
const DAY = 86_400_000;

const item = (overrides: Partial<Item> = {}): Item => ({
  id: "c1",
  kind: "comments",
  community: "AskReddit",
  title: "What was your worst job?",
  body: "A call center in 2016",
  url: "https://www.reddit.com/r/AskReddit/comments/x/",
  score: 12,
  createdAt: NOW - 400 * DAY,
  ...overrides,
});

const verdict = (rule: Partial<Rule>, target: Item) =>
  judge({ ...DEFAULT_RULE, ...rule }, NOW, "r/")(target);

describe("judge", () => {
  it("puts everything on the list by default", () => {
    expect(verdict({}, item())).toEqual({ type: "goes" });
  });

  it("keeps items with an unknown date or score off limited lists", () => {
    const undated = item({ createdAt: null, score: null });
    expect(verdict({}, undated)).toEqual({ type: "goes" });
    expect(verdict({ age: { type: "older", days: 30 } }, undated)).toEqual({
      type: "out",
    });
    expect(verdict({ maxScore: 100 }, undated)).toEqual({ type: "out" });
    expect(verdict({ keep: [{ type: "score", min: 500 }] }, undated)).toEqual({
      type: "kept",
      reason: "points unknown",
    });
  });

  it("filters by community, ignoring case", () => {
    expect(verdict({ communities: ["askreddit"] }, item())).toEqual({
      type: "goes",
    });
    expect(verdict({ communities: ["buildapc"] }, item())).toEqual({
      type: "out",
    });
  });

  it("keeps newer items out of an 'older than' sentence", () => {
    const rule: Partial<Rule> = { age: { type: "older", days: 365 } };
    expect(verdict(rule, item())).toEqual({ type: "goes" });
    expect(verdict(rule, item({ createdAt: NOW - 10 * DAY }))).toEqual({
      type: "out",
    });
  });

  it("treats a date range as from inclusive, to exclusive", () => {
    const from = Date.UTC(2025, 0, 1);
    const to = Date.UTC(2025, 1, 1);
    const rule: Partial<Rule> = { age: { type: "between", from, to } };
    expect(verdict(rule, item({ createdAt: from }))).toEqual({ type: "goes" });
    expect(verdict(rule, item({ createdAt: to }))).toEqual({ type: "out" });
  });

  it("only takes items under the score limit", () => {
    expect(verdict({ maxScore: 13 }, item())).toEqual({ type: "goes" });
    expect(verdict({ maxScore: 12 }, item())).toEqual({ type: "out" });
  });

  it("matches text in the body or the title", () => {
    expect(verdict({ text: "CALL CENTER" }, item())).toEqual({ type: "goes" });
    expect(verdict({ text: "worst job" }, item())).toEqual({ type: "goes" });
    expect(verdict({ text: "pizza" }, item())).toEqual({ type: "out" });
  });

  it("names the keep rule that saved an item", () => {
    expect(
      verdict({ keep: [{ type: "community", value: "askreddit" }] }, item()),
    ).toEqual({ type: "kept", reason: "r/askreddit" });
    expect(verdict({ keep: [{ type: "score", min: 10 }] }, item())).toEqual({
      type: "kept",
      reason: "over 10 points",
    });
    expect(
      verdict({ keep: [{ type: "text", value: "2016" }] }, item()),
    ).toEqual({
      type: "kept",
      reason: 'containing "2016"',
    });
  });

  it("keeps items picked by hand", () => {
    expect(verdict({ keepIds: ["c1"] }, item())).toEqual({
      type: "kept",
      reason: "kept by you",
    });
  });

  it("leaves items outside the sentence out, even with a keep rule", () => {
    expect(
      verdict(
        { communities: ["buildapc"], keep: [{ type: "score", min: 0 }] },
        item(),
      ),
    ).toEqual({ type: "out" });
  });
});

describe("ageLabel", () => {
  it("names presets and ranges", () => {
    expect(ageLabel({ type: "any" })).toBe("from any time");
    expect(ageLabel({ type: "older", days: 365 })).toBe("older than 1 year");
    expect(
      ageLabel({
        type: "between",
        from: new Date(2018, 2, 1).getTime(),
        to: new Date(2021, 6, 1).getTime(),
      }),
    ).toBe("from Mar 2018 to Jun 2021");
  });
});

describe("ruleSchema", () => {
  it("rejects a rule from a confused or hostile window", () => {
    expect(
      ruleSchema.safeParse({ ...DEFAULT_RULE, keep: [{ type: "all" }] })
        .success,
    ).toBe(false);
    expect(ruleSchema.safeParse(DEFAULT_RULE).success).toBe(true);
  });
});
