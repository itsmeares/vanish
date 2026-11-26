// Cleans recorded site traffic before it is kept anywhere. Anything that
// could sign someone in as you (cookies, tokens, passwords, session ids) or
// reach you (email, phone) is replaced. When unsure, it removes too much.

export const REMOVED = "[removed]";

// Matched against the words of a name, so `access_token` and `x-csrf-token`
// match but `author` does not.
const SECRET_WORDS = new Set([
  "auth",
  "authorization",
  "bearer",
  "code",
  "cookie",
  "cookies",
  "credential",
  "credentials",
  "csrf",
  "email",
  "jwt",
  "key",
  "loid",
  "modhash",
  "otp",
  "pass",
  "passwd",
  "password",
  "phone",
  "secret",
  "session",
  "sid",
  "signature",
  "token",
  "xsrf",
]);

export const isSecretName = (name: string) =>
  name
    .replace(/([a-z])([A-Z])/g, "$1 $2")
    .toLowerCase()
    .split(/[^a-z0-9]+/)
    .some((word) => SECRET_WORDS.has(word));

const JWT = /eyJ[\w-]{4,}\.[\w-]{4,}\.[\w-]{4,}/g;
const BEARER = /\b(Bearer|Basic)\s+[\w.~+/=-]+/gi;
const EMAIL = /[\w.+-]+@[\w-]+(\.[\w-]+)+/g;

const scrubText = (text: string) =>
  text
    .replace(JWT, REMOVED)
    .replace(BEARER, `$1 ${REMOVED}`)
    .replace(EMAIL, REMOVED);

/** Everything under a secret key goes, but the shape stays readable. */
const removeAll = (value: unknown): unknown => {
  if (Array.isArray(value)) return value.map(removeAll);
  if (value && typeof value === "object")
    return Object.fromEntries(
      Object.entries(value).map(([key, inner]) => [key, removeAll(inner)]),
    );
  return value === null || typeof value === "boolean" ? value : REMOVED;
};

const scrubJson = (value: unknown): unknown => {
  if (typeof value === "string") return scrubText(value);
  if (Array.isArray(value)) return value.map(scrubJson);
  if (value && typeof value === "object") {
    return Object.fromEntries(
      Object.entries(value).map(([key, inner]) => [
        key,
        isSecretName(key) ? removeAll(inner) : scrubJson(inner),
      ]),
    );
  }
  return value;
};

/** Cleans every value, keeping repeated keys and their order. */
const scrubParams = (params: URLSearchParams) =>
  new URLSearchParams(
    Array.from(params, ([key, value]) => [
      key,
      isSecretName(key) ? REMOVED : scrubText(value),
    ]),
  ).toString();

// Secrets inside HTML and scripts: `"token":"…"`, `token=…`, and
// `<input name="csrf_token" value="…">` with attributes in any order.
const QUOTED_PAIR = /(["'])([\w.-]+)\1(\s*:\s*)(["'])((?:\\.|(?!\4).)*)\4/g;
const ASSIGNED = /\b([\w.-]+)=([^&\s"'<>;]+)/g;
const INPUT = /<input\b[^>]*>/gi;
// One HTML attribute: quoted, unquoted, or bare, with spaces around "=".
const ATTRIBUTE =
  /([^\s"'<>/=]+)(?:\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'=<>`]+)))?/g;

const attributes = (tag: string) =>
  new Map(
    Array.from(tag.slice(6).matchAll(ATTRIBUTE), (match) => [
      match[1].toLowerCase(),
      match[2] ?? match[3] ?? match[4] ?? "",
    ]),
  );

const scrubInput = (tag: string) => {
  const found = attributes(tag);
  const type = (found.get("type") ?? "").toLowerCase();
  if (
    !isSecretName(found.get("name") ?? "") &&
    type !== "hidden" &&
    type !== "password"
  )
    return tag;
  return tag.replace(
    /(\bvalue\s*=\s*)(?:"[^"]*"|'[^']*'|[^\s"'=<>`]+)/i,
    `$1"${REMOVED}"`,
  );
};

const scrubMarkup = (text: string) =>
  scrubText(text)
    .replace(QUOTED_PAIR, (match, quote, key, colon, valueQuote) =>
      isSecretName(key)
        ? `${quote}${key}${quote}${colon}${valueQuote}${REMOVED}${valueQuote}`
        : match,
    )
    .replace(ASSIGNED, (match, key) =>
      isSecretName(key) ? `${key}=${REMOVED}` : match,
    )
    .replace(INPUT, scrubInput);

/** Cleans a request or response body, whatever its format. */
export const scrubBody = (body: string, contentType = "") => {
  const trimmed = body.trimStart();
  if (trimmed.startsWith("{") || trimmed.startsWith("[")) {
    try {
      return JSON.stringify(scrubJson(JSON.parse(body)));
    } catch {
      // Not JSON after all; clean it as text below.
    }
  }
  if (contentType.includes("application/x-www-form-urlencoded"))
    return scrubParams(new URLSearchParams(body));
  // Uploads and multipart forms are not needed to learn how a site works,
  // and their fields are easy to miss.
  if (contentType.includes("multipart/"))
    return `[multipart body removed, ${body.length} characters]`;
  return scrubMarkup(body);
};

export const scrubHeaders = (headers: Record<string, string>) =>
  Object.fromEntries(
    Object.entries(headers).map(([name, value]) => [
      name,
      isSecretName(name) ? REMOVED : scrubText(value),
    ]),
  );

export const scrubUrl = (raw: string) => {
  let url: URL;
  try {
    url = new URL(raw);
  } catch {
    return scrubMarkup(raw);
  }
  url.username = "";
  url.password = "";
  url.search = scrubParams(url.searchParams);
  url.hash = "";
  return scrubText(url.toString());
};
