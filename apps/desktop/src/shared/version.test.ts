import { expect, it } from "vite-plus/test";

import { isNewer } from "./version";

it("compares releases number by number", () => {
  expect(isNewer("v0.2.0", "0.1.9")).toBe(true);
  expect(isNewer("0.10.0", "0.9.0")).toBe(true);
  expect(isNewer("0.1.0", "0.1.0")).toBe(false);
  expect(isNewer("0.0.9", "0.1.0")).toBe(false);
});

it("ignores pre-releases but moves a pre-release install to its release", () => {
  expect(isNewer("0.2.0-rc.1", "0.1.0")).toBe(false);
  expect(isNewer("0.2.0", "0.2.0-rc.1")).toBe(true);
});
