export const KINDS = ["comments", "posts", "votes", "saved", "hidden"] as const;
export type Kind = (typeof KINDS)[number];

export type Platform = {
  id: "reddit";
  name: string;
  /** What the platform calls a community, and how it writes one. */
  community: { noun: string; prefix: string };
  loginUrl: string;
  homeUrl: string;
  /** Hosts the recording keeps. Everything else (ads, analytics) is dropped. */
  hosts: string[];
  /** `verb` starts the cleanup sentence; `done` names the finished action. */
  kinds: Record<
    Kind,
    {
      label: string;
      verb: string;
      doing: string;
      done: string;
      one: string;
      many: string;
    }
  >;
};

export const REDDIT: Platform = {
  id: "reddit",
  name: "Reddit",
  community: { noun: "subreddit", prefix: "r/" },
  loginUrl: "https://www.reddit.com/login/",
  homeUrl: "https://www.reddit.com/",
  hosts: ["reddit.com", "redd.it", "redditmedia.com"],
  kinds: {
    comments: {
      doing: "Deleting",
      label: "Comments",
      verb: "Delete",
      done: "deleted",
      one: "comment",
      many: "comments",
    },
    posts: {
      doing: "Deleting",
      label: "Posts",
      verb: "Delete",
      done: "deleted",
      one: "post",
      many: "posts",
    },
    votes: {
      doing: "Removing",
      label: "Votes",
      verb: "Remove",
      done: "removed",
      one: "vote",
      many: "votes",
    },
    saved: {
      doing: "Unsaving",
      label: "Saved",
      verb: "Unsave",
      done: "unsaved",
      one: "saved item",
      many: "saved items",
    },
    hidden: {
      doing: "Unhiding",
      label: "Hidden",
      verb: "Unhide",
      done: "unhidden",
      one: "hidden post",
      many: "hidden posts",
    },
  },
};

export const PLATFORMS = { reddit: REDDIT } as const;
export type PlatformId = keyof typeof PLATFORMS;

/** Kinds with text Vanish can overwrite before deleting. */
export const hasText = (kind: Kind) => kind === "comments" || kind === "posts";

export const keepsHost = (platform: Platform, url: string) => {
  let host: string;
  try {
    host = new URL(url).hostname;
  } catch {
    return false;
  }
  return platform.hosts.some(
    (kept) => host === kept || host.endsWith(`.${kept}`),
  );
};
