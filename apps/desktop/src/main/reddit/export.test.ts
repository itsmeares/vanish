import { strToU8, zipSync } from "fflate";
import { describe, expect, it } from "vite-plus/test";

import { parseCsv, readExport } from "./export";

const zip = (files: Record<string, string>) =>
  zipSync(
    Object.fromEntries(
      Object.entries(files).map(([name, text]) => [name, strToU8(text)]),
    ),
  );

describe("parseCsv", () => {
  it("reads quoted commas, quotes and line breaks", () => {
    expect(
      parseCsv('﻿id,body\r\na1,"one, ""two""\nthree"\r\nb2,plain\r\n'),
    ).toEqual([
      { id: "a1", body: 'one, "two"\nthree' },
      { id: "b2", body: "plain" },
    ]);
  });
});

describe("readExport", () => {
  const files = {
    "statistics.csv":
      "statistic,value\naccount name,quiet_harbor\nemail address,someone@example.com\n",
    "ip_logs.csv": "date,ip\n2026-01-01,192.0.2.1\n",
    "comments.csv":
      "id,permalink,date,ip,subreddit,gildings,link,parent,body,media\n" +
      'abc123,https://www.reddit.com/r/cooking/comments/p1/comment/abc123/,2022-04-08 14:36:44 UTC,,cooking,0,https://www.reddit.com/r/cooking/comments/p1,,"Salt it, like the sea",\n',
    "posts.csv":
      "id,permalink,date,ip,subreddit,gildings,title,url,body\n" +
      "p9,https://www.reddit.com/r/movies/comments/p9/the_sequel/,2020-10-17 10:22:44 UTC,,movies,0,The sequel was better,,\n",
    "post_votes.csv":
      "id,permalink,direction\n" +
      "v1,https://www.reddit.com/r/movies/comments/v1/a_great_trailer/,up\n" +
      "v2,https://www.reddit.com/r/movies/comments/v2/taken_back/,none\n",
    "comment_votes.csv":
      "id,permalink,direction\nc7,https://www.reddit.com/r/movies/comments/v1/a_great_trailer/c7/,down\n",
    "saved_posts.csv":
      "id,permalink\ns1,https://www.reddit.com/r/buildapc/comments/s1/final_build/\n",
    "hidden_posts.csv": "id,permalink\n",
  };

  it("reads every kind with Reddit's id prefixes", () => {
    const { account, items } = readExport(zip(files));
    expect(account).toBe("quiet_harbor");
    expect(items).toEqual([
      {
        id: "t1_abc123",
        kind: "comments",
        community: "cooking",
        title: "",
        body: "Salt it, like the sea",
        url: "https://www.reddit.com/r/cooking/comments/p1/comment/abc123/",
        score: null,
        createdAt: Date.UTC(2022, 3, 8, 14, 36, 44),
      },
      {
        id: "t3_p9",
        kind: "posts",
        community: "movies",
        title: "The sequel was better",
        body: "",
        url: "https://www.reddit.com/r/movies/comments/p9/the_sequel/",
        score: null,
        createdAt: Date.UTC(2020, 9, 17, 10, 22, 44),
      },
      {
        id: "t3_v1",
        kind: "votes",
        community: "movies",
        title: "a great trailer",
        body: "",
        url: "https://www.reddit.com/r/movies/comments/v1/a_great_trailer/",
        score: null,
        createdAt: null,
      },
      expect.objectContaining({ id: "t1_c7", kind: "votes" }),
      expect.objectContaining({
        id: "t3_s1",
        kind: "saved",
        community: "buildapc",
        title: "final build",
      }),
    ]);
  });

  it("finds the files inside a folder in the zip", () => {
    const nested = Object.fromEntries(
      Object.entries(files).map(([name, text]) => [`export/${name}`, text]),
    );
    expect(readExport(zip(nested)).items).toHaveLength(5);
  });

  it("refuses a zip that is not a Reddit export", () => {
    expect(() => readExport(zip({ "notes.txt": "hello" }))).toThrow(
      /not a Reddit data export/,
    );
  });
});
