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

const scrubForm = (form: string) => {
  const params = new URLSearchParams(form);
  for (const key of Array.from(params.keys())) {
    if (isSecretName(key)) params.set(key, REMOVED);
    else params.set(key, scrubText(params.get(key) ?? ""));
  }
  return params.toString();
};

// Secrets inside HTML and scripts: `"token":"…"`, `token=…`, and
// `<input name="csrf_token" value="…">` in either attribute order.
const QUOTED_PAIR = /(["'])([\w.-]+)\1(\s*:\s*)(["'])((?:\\.|(?!\4).)*)\4/g;
const ASSIGNED = /\b([\w.-]+)=([^&\s"'<>;]+)/g;
const INPUT = /<input\b[^>]*>/gi;

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
    .replace(INPUT, (tag) => {
      const name = /\bname=(["'])(.*?)\1/i.exec(tag)?.[2] ?? "";
      const type = /\btype=(["'])(.*?)\1/i.exec(tag)?.[2] ?? "";
      if (
        !isSecretName(name) &&
        type.toLowerCase() !== "hidden" &&
        type.toLowerCase() !== "password"
      )
        return tag;
      return tag.replace(/\bvalue=(["'])(.*?)\1/i, `value=$1${REMOVED}$1`);
    });

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
    return scrubForm(body);
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
  for (const key of Array.from(url.searchParams.keys())) {
    if (isSecretName(key)) url.searchParams.set(key, REMOVED);
  }
  url.hash = "";
  return scrubText(url.toString());
};
