// `pnpm desktop:demo` runs the real app on a throwaway data folder filled with
// made-up Reddit items, and removes them without talking to any site. It is
// for judging the app and testing the cleanup flow, never for real accounts.
import { setTimeout as sleep } from "node:timers/promises";

import type { Kind } from "../shared/platforms";
import type { Item } from "../shared/rule";
import type { Db } from "./db";
import type { Remover } from "./runner";

// Each community with its weight, one thread title and one thing people say there.
const COMMUNITIES = [
  [
    "AskReddit",
    9,
    "What was your worst job?",
    "the worst job I ever had was at a call center, they tracked bathroom breaks to the minute",
  ],
  [
    "personalfinance",
    4,
    "Should I move out at 24 or keep saving?",
    "I moved out at 24 and it was the best decision I made, even with the rent",
  ],
  [
    "buildapc",
    4,
    "Final build, thanks everyone for the help!",
    "check your PSU before anything else, mine was the reason for every crash",
  ],
  [
    "relationships",
    3,
    "Need advice: she wants space",
    "we talked and she said she needs space, I don't know what to do now",
  ],
  [
    "Turkey",
    2,
    "Kira artık maaşın yarısı, nasıl yaşıyorsunuz?",
    "rent in Istanbul is half of a salary now and nobody talks about it",
  ],
  [
    "cooking",
    3,
    "What's one cooking tip you swear by?",
    "salt the pasta water like the sea and stop rinsing it",
  ],
  [
    "movies",
    3,
    "Unpopular opinion: the sequel is better",
    "the sequel was better and I will die on this hill",
  ],
  [
    "learnprogramming",
    3,
    "How do I actually get good at programming?",
    "learn the basics of git before any framework, trust me",
  ],
  [
    "CasualUK",
    2,
    "What's the most British thing your family does?",
    "my nan still calls the internet 'the computer'",
  ],
  [
    "NoStupidQuestions",
    3,
    "Is it stupid to ask for a refund on a bank fee?",
    "you can ask the bank to waive the fee, they usually do",
  ],
  [
    "gaming",
    3,
    "The new patch is a disaster",
    "I've been playing it since launch and the last patch ruined it",
  ],
  [
    "MechanicalKeyboards",
    1,
    "Finally finished my first custom keyboard",
    "the switches were too loud for the office so I went back to browns",
  ],
] as const;

const OPENERS = [
  "Honestly",
  "Not gonna lie,",
  "Same here.",
  "This is the answer.",
  "Hard disagree,",
  "Update:",
  "Funny story,",
  "As someone who did this for years,",
  "Ok but",
  "I asked my landlord and",
];

const ENDINGS = [
  "",
  " Edit: thanks for the replies.",
  " Not sure why this is controversial.",
  " Anyway, good luck.",
  " Source: did it twice.",
];

const sentence = (opener: string, said: string, ending: string) =>
  `${opener} ${/[.:]$/.test(opener) ? said[0].toUpperCase() + said.slice(1) : said}.${ending}`;

/** The same numbers every time, so screenshots and checks are repeatable. */
const seeded = (seed: number) => () => {
  seed = (seed + 0x6d2b79f5) | 0;
  let value = Math.imul(seed ^ (seed >>> 15), 1 | seed);
  value = (value + Math.imul(value ^ (value >>> 7), 61 | value)) ^ value;
  return ((value ^ (value >>> 14)) >>> 0) / 4_294_967_296;
};

const pickWeighted = (random: () => number) => {
  const total = COMMUNITIES.reduce((sum, [, weight]) => sum + weight, 0);
  let roll = random() * total;
  for (const community of COMMUNITIES) {
    roll -= community[1];
    if (roll < 0) return community;
  }
  return COMMUNITIES[0];
};

const makeItems = (
  kind: Kind,
  count: number,
  random: () => number,
  now: number,
): Item[] => {
  const span = 8 * 365 * 86_400_000;
  return Array.from({ length: count }, (_, index) => {
    const [community, , title, said] = pickWeighted(random);
    const pick = <T>(list: readonly T[]) =>
      list[Math.floor(random() * list.length)];
    // A little more recent activity than old, like a real account.
    const age = span * random() ** 1.15;
    const id = `${kind.slice(0, 1)}${index.toString(36)}`;
    return {
      id,
      kind,
      community,
      title,
      body:
        kind === "comments" || kind === "posts"
          ? sentence(pick(OPENERS), said, pick(ENDINGS))
          : "",
      url: `https://www.reddit.com/r/${community}/comments/${id}/`,
      score: Math.round(2 ** (random() * 12)) - Math.round(random() * 4),
      createdAt: Math.round(now - age),
    };
  });
};

export const seedDemo = (db: Db) => {
  if (db.accounts().length > 0) return;
  const random = seeded(7);
  const now = Date.now();
  const main = db.addAccount("reddit", "u/quiet_harbor");
  const counts: [Kind, number][] = [
    ["comments", 3412],
    ["posts", 211],
    ["votes", 9870],
    ["saved", 340],
    ["hidden", 52],
  ];
  db.saveItems(
    main,
    counts.flatMap(([kind, count]) => makeItems(kind, count, random, now)),
  );
  const second = db.addAccount("reddit", "u/late_night_owl");
  db.saveItems(second, [
    ...makeItems("comments", 148, random, now),
    ...makeItems("posts", 9, random, now),
  ]);
};

/** Pretends to remove items: a short wait, then success. */
export const demoRemover: Remover = { remove: () => sleep(350) };
