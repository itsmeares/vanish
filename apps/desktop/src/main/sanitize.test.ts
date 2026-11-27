import { describe, expect, it } from "vite-plus/test";

import {
  isSecretName,
  markCookies,
  REMOVED,
  scrubBody,
  scrubHeaders,
  scrubUrl,
} from "./sanitize";

const JWT =
  "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJ0X2FiYzEyMyIsImV4cCI6MTc5MDAwMDAwMH0.c2lnbmF0dXJlLXZhbHVl";

describe("isSecretName", () => {
  it("matches the words of a name", () => {
    for (const name of [
      "cookie",
      "Set-Cookie",
      "authorization",
      "x-csrf-token",
      "access_token",
      "accessToken",
      "reddit_session",
      "password",
      "loid",
      "api_key",
      "email",
    ])
      expect(isSecretName(name), name).toBe(true);
  });

  it("leaves ordinary fields alone", () => {
    for (const name of [
      "author",
      "subreddit",
      "body",
      "keyboard",
      "title",
      "permalink",
      "score",
    ])
      expect(isSecretName(name), name).toBe(false);
  });
});

describe("scrubHeaders", () => {
  it("removes cookies, auth headers and tokens, keeps the rest", () => {
    expect(
      scrubHeaders({
        Cookie: "reddit_session=abc; token_v2=xyz",
        "set-cookie":
          "token_v2=xyz; Path=/; HttpOnly\ncsrf_token=def; SameSite=Strict",
        Authorization: "Bearer abc.def",
        "x-reddit-loid": "000000abc",
        "content-type": "application/json",
        "x-custom": `Bearer ${JWT}`,
      }),
    ).toEqual({
      Cookie: `reddit_session=${REMOVED}; token_v2=${REMOVED}`,
      "set-cookie": `token_v2=${REMOVED}; Path=/; HttpOnly\ncsrf_token=${REMOVED}; SameSite=Strict`,
      Authorization: REMOVED,
      "x-reddit-loid": REMOVED,
      "content-type": "application/json",
      "x-custom": `Bearer ${REMOVED}`,
    });
  });
});

describe("scrubBody", () => {
  it("removes secret keys at any depth of JSON and keeps the shape", () => {
    const body = JSON.stringify({
      variables: { input: { password: "hunter2", username: "quiet_harbor" } },
      data: {
        session: { id: "abc", active: true },
        author: "quiet_harbor",
        note: JWT,
      },
      list: [{ csrf_token: "def" }],
      cookies: ["a=1", "b=2"],
    });
    expect(JSON.parse(scrubBody(body))).toEqual({
      variables: { input: { password: REMOVED, username: "quiet_harbor" } },
      data: {
        session: { id: REMOVED, active: true },
        author: "quiet_harbor",
        note: REMOVED,
      },
      list: [{ csrf_token: REMOVED }],
      cookies: [REMOVED, REMOVED],
    });
  });

  it("removes secret fields from a login form", () => {
    const form =
      "username=quiet_harbor&password=hunter2&csrf_token=abc&otp=123456";
    const clean = new URLSearchParams(
      scrubBody(form, "application/x-www-form-urlencoded"),
    );
    expect(clean.get("username")).toBe("quiet_harbor");
    expect(clean.get("password")).toBe(REMOVED);
    expect(clean.get("csrf_token")).toBe(REMOVED);
    expect(clean.get("otp")).toBe(REMOVED);
  });

  it("removes secrets inside a page's HTML and scripts", () => {
    const html = `
      <input type="hidden" name="csrf_token" value="a1b2c3">
      <input value="secret-first" name="session_tracker" type="text">
      <input type="text" name="q" value="cats">
      <script>window.___r = {"accessToken":"${JWT}","user":{"name":"quiet_harbor"},'apiKey': 'k-123'};</script>
      <a href="/r/cats?token=abc&sort=new">cats</a>
      <p>Contact me at someone@example.com</p>`;
    const clean = scrubBody(html, "text/html");
    expect(clean).not.toContain("a1b2c3");
    expect(clean).not.toContain("secret-first");
    expect(clean).not.toContain(JWT);
    expect(clean).not.toContain("k-123");
    expect(clean).not.toContain("token=abc");
    expect(clean).not.toContain("someone@example.com");
    expect(clean).toContain('value="cats"');
    expect(clean).toContain('"name":"quiet_harbor"');
    expect(clean).toContain("sort=new");
  });

  it("finds input values however the attributes are written", () => {
    const html = `<input type = "password" name = "password" value = "hunter2">
      <input name=csrf_token value=abc123 type=hidden>
      <input type=text name=q value=cats>`;
    const clean = scrubBody(html, "text/html");
    expect(clean).not.toContain("hunter2");
    expect(clean).not.toContain("abc123");
    expect(clean).toContain("value=cats");
  });

  it("drops multipart bodies, whose fields are easy to miss", () => {
    const body =
      '--x\r\nContent-Disposition: form-data; name="password"\r\n\r\nhunter2\r\n--x--';
    expect(scrubBody(body, "multipart/form-data; boundary=x")).not.toContain(
      "hunter2",
    );
  });

  it("removes tokens in links inside JSON text", () => {
    const body = JSON.stringify({
      referrer: { url: "https://www.reddit.com/login/?jsc_token=abc123&x=1" },
    });
    expect(JSON.parse(scrubBody(body)).referrer.url).toBe(
      `https://www.reddit.com/login/?jsc_token=${REMOVED}&x=1`,
    );
  });

  it("cleans text that only looks like JSON", () => {
    expect(scrubBody(`{not json "token":"abc"}`)).toBe(
      `{not json "token":"${REMOVED}"}`,
    );
  });
});

describe("markCookies", () => {
  const cookies = [
    { name: "csrf_token", value: "c5f1e2d3a4b5" },
    { name: "theme", value: "1" },
  ];

  it("names the cookie a removed secret came from", () => {
    const body = markCookies(
      JSON.stringify({ operation: "DeletePost", csrf_token: "c5f1e2d3a4b5" }),
      cookies,
    );
    expect(JSON.parse(scrubBody(body)).csrf_token).toBe("[cookie:csrf_token]");
    expect(
      scrubUrl(markCookies("https://x.test/?token=c5f1e2d3a4b5", cookies)),
    ).toBe(
      `https://x.test/?token=${encodeURIComponent("[cookie:csrf_token]")}`,
    );
  });

  it("leaves short values alone and still removes unknown secrets", () => {
    const body = markCookies(
      JSON.stringify({ theme: "1", token: "zzz" }),
      cookies,
    );
    expect(JSON.parse(scrubBody(body))).toEqual({ theme: "1", token: REMOVED });
  });
});

describe("scrubUrl", () => {
  it("cleans encoded values and keeps repeated keys", () => {
    expect(
      scrubUrl("https://www.reddit.com/search?q=someone%40example.com&t=a&t=b"),
    ).toBe(
      `https://www.reddit.com/search?q=${encodeURIComponent(REMOVED)}&t=a&t=b`,
    );
  });

  it("removes secret query values, sign-in parts and the fragment", () => {
    const clean = scrubUrl(
      "https://user:pw@www.reddit.com/api/v1/me?raw_json=1&access_token=abc#token=xyz",
    );
    expect(clean).toBe(
      `https://www.reddit.com/api/v1/me?raw_json=1&access_token=${encodeURIComponent(REMOVED)}`,
    );
  });
});
