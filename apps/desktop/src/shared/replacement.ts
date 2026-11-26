export const SITE_URL = "https://vanish.itsmeares.com";
export const DEFAULT_TEXT = `This was removed with [Vanish](${SITE_URL})`;

// Plain, neutral words. A few of them start each replacement, so a thousand
// overwritten comments do not all read the same.
const WORDS =
  "amber anchor apple arch autumn badge bamboo basket beacon birch blanket bloom border breeze brick bridge bubble cabin candle canyon carpet cedar chalk cherry cliff clover cobalt comet copper coral cotton crane crystal dawn delta desert dune echo ember fable feather fern field flint forest fountain garden glacier granite gravel harbor hazel horizon island ivory jasmine kettle lagoon lantern lemon linen maple marble meadow mirror moss mountain needle oasis ocean olive orbit orchard paddle pebble pepper pillow pine planet pocket prairie quartz quill rain river rocket saddle sail salt shadow shell silver slate spark spruce stone summit tide timber tulip valley velvet violet walnut willow window winter".split(
    " ",
  );

export type ReplacementSettings = { text: string; randomWords: boolean };

/** The text that replaces a comment or post before it is deleted. */
export const replacementText = (
  settings: ReplacementSettings,
  random: () => number = Math.random,
) => {
  const count = 3 + Math.floor(random() * 4);
  const words = settings.randomWords
    ? Array.from(
        { length: count },
        () => WORDS[Math.floor(random() * WORDS.length)],
      )
    : [];
  return [words.join(" "), settings.text.trim()].filter(Boolean).join("\n\n");
};
