import { describe, expect, it } from "vite-plus/test";

import { DEFAULT_TEXT, replacementText } from "./replacement";

describe("replacementText", () => {
  it("starts with a few random words, then the signature with its link", () => {
    const text = replacementText({ text: DEFAULT_TEXT, randomWords: true });
    const [words, signature] = text.split("\n\n");
    expect(words.split(" ").length).toBeGreaterThanOrEqual(3);
    expect(words.split(" ").length).toBeLessThanOrEqual(6);
    expect(signature).toBe(
      "This was removed with [Vanish](https://vanish.itsmeares.com)",
    );
  });

  it("differs between items", () => {
    const texts = new Set(
      Array.from({ length: 20 }, () =>
        replacementText({ text: DEFAULT_TEXT, randomWords: true }),
      ),
    );
    expect(texts.size).toBeGreaterThan(1);
  });

  it("can be only the signature, or blank", () => {
    expect(replacementText({ text: " Gone. ", randomWords: false })).toBe(
      "Gone.",
    );
    expect(replacementText({ text: "", randomWords: false })).toBe("");
  });
});
