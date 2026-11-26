import type { WebContents } from "electron";

import { keepsHost, type Platform } from "../shared/platforms";
import { scrubBody, scrubHeaders, scrubUrl } from "./sanitize";

/** Only page loads and data requests. Images, fonts, styles and scripts say nothing about how the site works. */
const KEPT_TYPES = new Set(["Document", "XHR", "Fetch", "EventSource"]);
const MAX_BODY = 2_000_000;

type Entry = {
  time: string;
  type: string;
  method: string;
  url: string;
  status: number | null;
  requestHeaders: Record<string, string>;
  requestBody: string | null;
  responseHeaders: Record<string, string>;
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
  headers: Record<string, string>;
  postData: string | null;
  hasPostData: boolean;
  status: number | null;
  responseHeaders: Record<string, string>;
  mimeType: string | null;
};

const clean = (
  pending: Pending,
  body: string | null,
  error?: string,
): Entry => {
  const contentType =
    pending.headers["Content-Type"] ?? pending.headers["content-type"] ?? "";
  return {
    time: pending.time,
    type: pending.type,
    method: pending.method,
    url: scrubUrl(pending.url),
    status: pending.status,
    requestHeaders: scrubHeaders(pending.headers),
    requestBody:
      pending.postData === null
        ? null
        : scrubBody(pending.postData, contentType),
    responseHeaders: scrubHeaders(pending.responseHeaders),
    mimeType: pending.mimeType,
    responseBody:
      body === null ? null : scrubBody(body, pending.mimeType ?? ""),
    ...(error ? { error } : {}),
  };
};

type CdpRequest = {
  url: string;
  method: string;
  headers: Record<string, string>;
  postData?: string;
  hasPostData?: boolean;
};
type CdpResponse = {
  status: number;
  headers: Record<string, string>;
  mimeType: string;
};

export const createRecorder = (
  platform: Platform,
  onCount: (count: number) => void,
) => {
  const entries: Entry[] = [];
  const startedAt = new Date().toISOString();
  const attached = new Set<WebContents>();

  const add = (entry: Entry) => {
    entries.push(entry);
    onCount(entries.length);
  };

  const attach = (contents: WebContents) => {
    if (attached.has(contents)) return;
    const cdp = contents.debugger;
    const pending = new Map<string, Pending>();
    cdp.attach("1.3");
    attached.add(contents);
    contents.once("destroyed", () => attached.delete(contents));

    const command = <T>(method: string, params: object) =>
      cdp.sendCommand(method, params) as Promise<T>;

    cdp.on("message", (_event, method, params) => {
      void (async () => {
        if (method === "Network.requestWillBeSent") {
          const { requestId, request, type, redirectResponse } = params as {
            requestId: string;
            request: CdpRequest;
            type?: string;
            redirectResponse?: CdpResponse;
          };
          const previous = pending.get(requestId);
          if (previous && redirectResponse) {
            previous.status = redirectResponse.status;
            previous.responseHeaders = redirectResponse.headers;
            add(clean(previous, null));
          }
          pending.delete(requestId);
          if (!KEPT_TYPES.has(type ?? "") || !keepsHost(platform, request.url))
            return;
          pending.set(requestId, {
            time: new Date().toISOString(),
            type: type ?? "Other",
            method: request.method,
            url: request.url,
            headers: request.headers,
            postData: request.postData ?? null,
            hasPostData: Boolean(request.hasPostData),
            status: null,
            responseHeaders: {},
            mimeType: null,
          });
        } else if (method === "Network.responseReceived") {
          const { requestId, response } = params as {
            requestId: string;
            response: CdpResponse;
          };
          const request = pending.get(requestId);
          if (!request) return;
          request.status = response.status;
          request.responseHeaders = response.headers;
          request.mimeType = response.mimeType;
        } else if (method === "Network.loadingFinished") {
          const { requestId } = params as { requestId: string };
          const request = pending.get(requestId);
          if (!request) return;
          pending.delete(requestId);
          if (request.hasPostData && request.postData === null) {
            request.postData = await command<{ postData: string }>(
              "Network.getRequestPostData",
              {
                requestId,
              },
            )
              .then((result) => result.postData)
              .catch(() => null);
          }
          const body = await command<{ body: string; base64Encoded: boolean }>(
            "Network.getResponseBody",
            { requestId },
          )
            .then(({ body, base64Encoded }) =>
              base64Encoded
                ? `[binary, ${Math.round((body.length * 3) / 4)} bytes]`
                : body.length > MAX_BODY
                  ? `[too large, ${body.length} characters]`
                  : body,
            )
            .catch(() => null);
          add(clean(request, body));
        } else if (method === "Network.loadingFailed") {
          const { requestId, errorText } = params as {
            requestId: string;
            errorText: string;
          };
          const request = pending.get(requestId);
          if (!request) return;
          pending.delete(requestId);
          add(clean(request, null, errorText));
        }
      })();
    });

    void command("Network.enable", { maxPostDataSize: MAX_BODY });
  };

  return {
    attach,
    count: () => entries.length,
    stop: () => {
      for (const contents of attached) {
        if (!contents.isDestroyed() && contents.debugger.isAttached())
          contents.debugger.detach();
      }
      attached.clear();
      return {
        format: "vanish-recording",
        version: 1,
        platform: platform.id,
        startedAt,
        stoppedAt: new Date().toISOString(),
        entries,
      };
    },
  };
};
