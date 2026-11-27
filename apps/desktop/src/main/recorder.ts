import type { WebContents } from "electron";

import { records, type Platform } from "../shared/platforms";
import { markCookies, scrubBody, scrubHeaders, scrubUrl } from "./sanitize";

/** Only page loads and data requests. Images, fonts, styles and scripts say nothing about how the site works. */
const KEPT_TYPES = new Set(["Document", "XHR", "Fetch", "EventSource"]);
const MAX_BODY = 2_000_000;
const BINARY = /^(image|video|audio|font)\/|octet-stream/;

type Headers = Record<string, string>;
type Cookie = { name: string; value: string };

type Entry = {
  time: string;
  type: string;
  method: string;
  url: string;
  status: number | null;
  requestHeaders: Headers;
  requestBody: string | null;
  responseHeaders: Headers;
  mimeType: string | null;
  responseBody: string | null;
  error?: string;
};

// The raw request exists only in memory until it is cleaned into an Entry.
type Pending = {
  time: string;
  type: string;
  method: string;
  url: string;
  headers: Headers;
  postData: string | null;
  hasPostData: boolean;
  status: number | null;
  responseHeaders: Headers;
  mimeType: string | null;
  // Some pages arrive as a stream that the page cuts short once it has what
  // it needs, so the body is collected as it arrives, not asked for at the end.
  chunks: Buffer[];
  streaming: Promise<boolean> | null;
};

const header = (headers: Headers, name: string) =>
  Object.entries(headers).find(([key]) => key.toLowerCase() === name)?.[1] ??
  "";

const readable = (mimeType: string | null, text: string, bytes: number) =>
  BINARY.test(mimeType ?? "")
    ? `[binary, ${bytes} bytes]`
    : text.length > MAX_BODY
      ? `[too large, ${text.length} characters]`
      : text;

const collected = (pending: Pending) => {
  const bytes = Buffer.concat(pending.chunks);
  return readable(pending.mimeType, bytes.toString("utf8"), bytes.length);
};

const clean = (
  pending: Pending,
  body: string | null,
  cookies: Cookie[],
  error?: string,
): Entry => {
  // Cookie values become `[cookie:name]` before cleaning, so the file shows
  // where a token came from and never keeps the token.
  const mark = (text: string) => markCookies(text, cookies);
  const markAll = (headers: Headers) =>
    Object.fromEntries(
      Object.entries(headers).map(([name, value]) => [name, mark(value)]),
    );
  return {
    time: pending.time,
    type: pending.type,
    method: pending.method,
    url: scrubUrl(mark(pending.url)),
    status: pending.status,
    requestHeaders: scrubHeaders(markAll(pending.headers)),
    requestBody:
      pending.postData === null
        ? null
        : scrubBody(
            mark(pending.postData),
            header(pending.headers, "content-type"),
          ),
    responseHeaders: scrubHeaders(markAll(pending.responseHeaders)),
    mimeType: pending.mimeType,
    responseBody:
      body === null ? null : scrubBody(mark(body), pending.mimeType ?? ""),
    ...(error ? { error } : {}),
  };
};

type CdpRequest = {
  url: string;
  method: string;
  headers: Headers;
  postData?: string;
  hasPostData?: boolean;
};
type CdpResponse = {
  status: number;
  headers: Headers;
  mimeType: string;
};

export const createRecorder = (
  platform: Platform,
  onCount: (count: number) => void,
) => {
  const entries: Entry[] = [];
  const startedAt = new Date().toISOString();
  // Each browser this recorder listens to, with its listener, so stopping
  // removes exactly this recorder's listener from a reused browser.
  const attached = new Map<WebContents, () => void>();
  let stopped = false;

  const add = (entry: Entry) => {
    // Requests still finishing when you stop are not part of the file.
    if (stopped) return;
    entries.push(entry);
    onCount(entries.length);
  };

  /** Resolves once the browser reports its traffic, so a page loaded after it is seen whole. */
  const attach = async (contents: WebContents) => {
    if (stopped || attached.has(contents)) return;
    const cdp = contents.debugger;
    const pending = new Map<string, Pending>();
    // The headers Chromium really sent and received (with cookies, origin and
    // fetch metadata) arrive in separate events, before or after the request.
    const sentHeaders = new Map<string, Headers>();
    const receivedHeaders = new Map<string, Headers>();
    cdp.attach("1.3");
    contents.once("destroyed", () => attached.delete(contents));

    const command = <T>(method: string, params: object) =>
      cdp.sendCommand(method, params) as Promise<T>;

    const cookies = () =>
      contents.session.cookies
        .get({})
        .then((list) => list.map(({ name, value }) => ({ name, value })))
        .catch(() => []);

    const finish = async (
      requestId: string,
      request: Pending,
      body: string | null,
      error?: string,
    ) => {
      sentHeaders.delete(requestId);
      receivedHeaders.delete(requestId);
      add(clean(request, body, await cookies(), error));
    };

    const listener = (_event: unknown, method: string, params: unknown) => {
      void (async () => {
        if (method === "Network.requestWillBeSent") {
          const { requestId, request, type, redirectResponse } = params as {
            requestId: string;
            request: CdpRequest;
            type?: string;
            redirectResponse?: CdpResponse;
          };
          const previous = pending.get(requestId);
          pending.delete(requestId);
          if (previous && redirectResponse) {
            previous.status = redirectResponse.status;
            previous.responseHeaders =
              receivedHeaders.get(requestId) ?? redirectResponse.headers;
            void finish(requestId, previous, null);
          }
          if (!KEPT_TYPES.has(type ?? "") || !records(platform, request.url))
            return;
          pending.set(requestId, {
            time: new Date().toISOString(),
            type: type ?? "Other",
            method: request.method,
            url: request.url,
            headers: sentHeaders.get(requestId) ?? request.headers,
            postData: request.postData ?? null,
            hasPostData: Boolean(request.hasPostData),
            status: null,
            responseHeaders: {},
            mimeType: null,
            chunks: [],
            streaming: null,
          });
        } else if (method === "Network.requestWillBeSentExtraInfo") {
          const { requestId, headers } = params as {
            requestId: string;
            headers: Headers;
          };
          const request = pending.get(requestId);
          if (request) request.headers = headers;
          else sentHeaders.set(requestId, headers);
        } else if (method === "Network.responseReceivedExtraInfo") {
          const { requestId, headers } = params as {
            requestId: string;
            headers: Headers;
          };
          const request = pending.get(requestId);
          if (request && request.status !== null)
            request.responseHeaders = headers;
          else receivedHeaders.set(requestId, headers);
        } else if (method === "Network.responseReceived") {
          const { requestId, response } = params as {
            requestId: string;
            response: CdpResponse;
          };
          const request = pending.get(requestId);
          if (!request) return;
          request.status = response.status;
          request.responseHeaders =
            receivedHeaders.get(requestId) ?? response.headers;
          request.mimeType = response.mimeType;
          if (BINARY.test(response.mimeType)) return;
          request.streaming = command<{ bufferedData: string }>(
            "Network.streamResourceContent",
            { requestId },
          )
            .then(({ bufferedData }) => {
              request.chunks.unshift(Buffer.from(bufferedData, "base64"));
              return true;
            })
            // Already finished: the body is asked for at the end instead.
            .catch(() => false);
        } else if (method === "Network.dataReceived") {
          const { requestId, data } = params as {
            requestId: string;
            data?: string;
          };
          if (data)
            pending.get(requestId)?.chunks.push(Buffer.from(data, "base64"));
        } else if (method === "Network.loadingFinished") {
          const { requestId } = params as { requestId: string };
          const request = pending.get(requestId);
          if (!request) {
            sentHeaders.delete(requestId);
            receivedHeaders.delete(requestId);
            return;
          }
          pending.delete(requestId);
          if (request.hasPostData && request.postData === null) {
            request.postData = await command<{ postData: string }>(
              "Network.getRequestPostData",
              { requestId },
            )
              .then((result) => result.postData)
              .catch(() => null);
          }
          const body = (await request.streaming)
            ? collected(request)
            : await command<{ body: string; base64Encoded: boolean }>(
                "Network.getResponseBody",
                { requestId },
              )
                .then(({ body, base64Encoded }) =>
                  base64Encoded
                    ? `[binary, ${Math.round((body.length * 3) / 4)} bytes]`
                    : readable(request.mimeType, body, body.length),
                )
                .catch(() => null);
          await finish(requestId, request, body);
        } else if (method === "Network.loadingFailed") {
          const { requestId, errorText } = params as {
            requestId: string;
            errorText: string;
          };
          const request = pending.get(requestId);
          if (!request) {
            sentHeaders.delete(requestId);
            receivedHeaders.delete(requestId);
            return;
          }
          pending.delete(requestId);
          // A page that stops a load early still got the part it read.
          const body =
            (await request.streaming) && request.chunks.length > 0
              ? collected(request)
              : null;
          await finish(requestId, request, body, errorText);
        }
      })();
    };
    cdp.on("message", listener);
    attached.set(contents, () => {
      cdp.off("message", listener);
      pending.clear();
      sentHeaders.clear();
      receivedHeaders.clear();
      if (!contents.isDestroyed() && cdp.isAttached()) cdp.detach();
    });

    // Never hold up the account window for long, even if the browser is slow to answer.
    await Promise.race([
      command("Network.enable", { maxPostDataSize: MAX_BODY }),
      new Promise((resolve) => setTimeout(resolve, 3000)),
    ]);
  };

  return {
    attach,
    count: () => entries.length,
    stop: () => {
      stopped = true;
      for (const detach of attached.values()) detach();
      attached.clear();
      return {
        format: "vanish-recording",
        version: 2,
        platform: platform.id,
        startedAt,
        stoppedAt: new Date().toISOString(),
        entries,
      };
    },
  };
};
