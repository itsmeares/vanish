import { Button } from "@vanish/ui/components/button";
import { Checkbox } from "@vanish/ui/components/checkbox";
import { Input } from "@vanish/ui/components/input";
import {
  Menu,
  MenuPopup,
  MenuRadioGroup,
  MenuRadioItem,
  MenuTrigger,
} from "@vanish/ui/components/menu";
import {
  Popover,
  PopoverPopup,
  PopoverTrigger,
} from "@vanish/ui/components/popover";
import {
  ToggleGroup,
  ToggleGroupItem,
} from "@vanish/ui/components/toggle-group";
import { PlusIcon, XIcon } from "lucide-react";
import { useMemo, useState, type ReactNode } from "react";

import { KINDS, type Kind, type Platform } from "../shared/platforms";
import {
  ageLabel,
  keepLabel,
  type Item,
  type KeepRule,
  type Rule,
} from "../shared/rule";
import { compact, count } from "./format";
import { TimeChart } from "./time-chart";

type Props = {
  platform: Platform;
  kind: Kind;
  counts: Record<Kind, number>;
  items: Item[];
  rule: Rule;
  disabled: boolean;
  onKind: (kind: Kind) => void;
  onRule: (rule: Rule) => void;
};

const Part = ({
  label,
  disabled,
  children,
  wide,
}: {
  label: ReactNode;
  disabled: boolean;
  children: ReactNode;
  wide?: boolean;
}) => (
  <Popover>
    <PopoverTrigger
      render={<Button variant="sentence" size="sentence" disabled={disabled} />}
    >
      {label}
    </PopoverTrigger>
    <PopoverPopup align="start" className={wide ? "w-[38rem]" : "w-80"}>
      {children}
    </PopoverPopup>
  </Popover>
);

const PresetRow = ({ children }: { children: ReactNode }) => (
  <div className="flex flex-wrap gap-1.5">{children}</div>
);

const CommunityPicker = ({
  platform,
  items,
  rule,
  onRule,
}: Omit<Props, "kind" | "counts" | "onKind" | "disabled">) => {
  const [search, setSearch] = useState("");
  const communities = useMemo(() => {
    const tally = new Map<string, number>();
    for (const item of items)
      tally.set(item.community, (tally.get(item.community) ?? 0) + 1);
    return [...tally].sort((a, b) => b[1] - a[1]);
  }, [items]);
  const chosen = new Set(rule.communities);
  const shown = communities.filter(([name]) =>
    name.toLowerCase().includes(search.toLowerCase()),
  );
  const toggle = (name: string) =>
    onRule({
      ...rule,
      communities: chosen.has(name)
        ? rule.communities.filter((value) => value !== name)
        : [...rule.communities, name],
    });
  return (
    <div className="flex flex-col gap-3">
      <Input
        placeholder={`Search ${platform.community.noun}s`}
        value={search}
        onChange={(event) => setSearch(event.target.value)}
        autoFocus
      />
      <div className="-mx-1 max-h-72 scrollbar-thin overflow-y-auto">
        {shown.map(([name, total]) => (
          <label
            key={name}
            className="flex cursor-pointer items-center gap-2.5 rounded-md px-1 py-1.5 hover:bg-hover"
          >
            <Checkbox
              checked={chosen.has(name)}
              onCheckedChange={() => toggle(name)}
            />
            <span className="flex-1 truncate">
              {platform.community.prefix}
              {name}
            </span>
            <span className="text-meta text-muted-foreground">
              {count(total)}
            </span>
          </label>
        ))}
        {shown.length === 0 && (
          <p className="px-1 py-2 text-meta text-muted-foreground">
            No {platform.community.noun} matches.
          </p>
        )}
      </div>
      <Button
        variant="outline"
        size="sm"
        disabled={rule.communities.length === 0}
        onClick={() => onRule({ ...rule, communities: [] })}
      >
        Any {platform.community.noun}
      </Button>
    </div>
  );
};

const SCORE_PRESETS = [10, 100, 500, 1000];

const ScorePicker = ({ rule, onRule }: Pick<Props, "rule" | "onRule">) => (
  <div className="flex flex-col gap-3">
    <p className="text-meta text-muted-foreground">
      Only items with fewer points than this go on the list.
    </p>
    <Input
      type="number"
      placeholder="Any score"
      value={rule.maxScore ?? ""}
      onChange={(event) => {
        const value = Number.parseInt(event.target.value, 10);
        onRule({ ...rule, maxScore: Number.isNaN(value) ? null : value });
      }}
      autoFocus
    />
    <PresetRow>
      {SCORE_PRESETS.map((preset) => (
        <Button
          key={preset}
          variant={rule.maxScore === preset ? "secondary" : "outline"}
          size="sm"
          onClick={() => onRule({ ...rule, maxScore: preset })}
        >
          Under {compact(preset)}
        </Button>
      ))}
      <Button
        variant={rule.maxScore === null ? "secondary" : "outline"}
        size="sm"
        onClick={() => onRule({ ...rule, maxScore: null })}
      >
        Any score
      </Button>
    </PresetRow>
  </div>
);

const TextPicker = ({ rule, onRule }: Pick<Props, "rule" | "onRule">) => (
  <div className="flex flex-col gap-3">
    <p className="text-meta text-muted-foreground">
      Only items that contain this text, in the title or the body.
    </p>
    <Input
      placeholder="Any text"
      value={rule.text}
      onChange={(event) =>
        onRule({ ...rule, text: event.target.value.slice(0, 200) })
      }
      autoFocus
    />
    <Button
      variant="outline"
      size="sm"
      disabled={!rule.text}
      onClick={() => onRule({ ...rule, text: "" })}
    >
      Any text
    </Button>
  </div>
);

const KeepAdder = ({
  platform,
  rule,
  onRule,
  disabled,
}: Pick<Props, "platform" | "rule" | "onRule" | "disabled">) => {
  const [type, setType] = useState<KeepRule["type"]>("community");
  const [value, setValue] = useState("");
  const [open, setOpen] = useState(false);
  const trimmed = value
    .trim()
    .replace(new RegExp(`^${platform.community.prefix}`, "i"), "");
  const score = Number.parseInt(trimmed, 10);
  const valid = type === "score" ? !Number.isNaN(score) : trimmed.length > 0;
  const add = () => {
    if (!valid) return;
    const keep: KeepRule =
      type === "score"
        ? { type, min: score }
        : { type, value: trimmed.slice(0, type === "text" ? 200 : 100) };
    onRule({ ...rule, keep: [...rule.keep, keep] });
    setValue("");
    setOpen(false);
  };
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        render={
          <Button
            variant="ghost-muted"
            size="sm"
            className="align-middle"
            disabled={disabled}
          />
        }
      >
        <PlusIcon />
        Add a keep rule
      </PopoverTrigger>
      <PopoverPopup align="start" className="w-80">
        <form
          className="flex flex-col gap-3"
          onSubmit={(event) => {
            event.preventDefault();
            add();
          }}
        >
          <ToggleGroup
            value={[type]}
            onValueChange={(next) =>
              next[0] && setType(next[0] as KeepRule["type"])
            }
            variant="outline"
            size="sm"
          >
            <ToggleGroupItem value="community">
              {platform.community.noun.replace(/^./, (c) => c.toUpperCase())}
            </ToggleGroupItem>
            <ToggleGroupItem value="score">Points</ToggleGroupItem>
            <ToggleGroupItem value="text">Text</ToggleGroupItem>
          </ToggleGroup>
          <Input
            autoFocus
            type={type === "score" ? "number" : "text"}
            placeholder={
              type === "community"
                ? `${platform.community.prefix}buildapc`
                : type === "score"
                  ? "Keep anything over this many points"
                  : "Keep anything containing this"
            }
            value={value}
            onChange={(event) => setValue(event.target.value)}
          />
          <Button type="submit" disabled={!valid}>
            Keep these
          </Button>
        </form>
      </PopoverPopup>
    </Popover>
  );
};

export const Sentence = (props: Props) => {
  const { platform, kind, counts, items, rule, disabled, onKind, onRule } =
    props;
  const names = platform.kinds[kind];
  const communities =
    rule.communities.length === 0
      ? `any ${platform.community.noun}`
      : rule.communities.length <= 2
        ? rule.communities
            .map((name) => `${platform.community.prefix}${name}`)
            .join(" or ")
        : `${rule.communities.length} ${platform.community.noun}s`;

  return (
    <div className="text-sentence text-foreground">
      <p className="max-w-[46rem] text-pretty">
        {names.verb} my{" "}
        <Menu>
          <MenuTrigger
            render={
              <Button variant="sentence" size="sentence" disabled={disabled} />
            }
          >
            {names.many}
          </MenuTrigger>
          <MenuPopup align="start">
            <MenuRadioGroup
              value={kind}
              onValueChange={(value) => onKind(value as Kind)}
            >
              {KINDS.map((option) => (
                <MenuRadioItem key={option} value={option}>
                  <span className="flex-1">{platform.kinds[option].many}</span>
                  <span className="ms-6 text-meta text-muted-foreground">
                    {count(counts[option])}
                  </span>
                </MenuRadioItem>
              ))}
            </MenuRadioGroup>
          </MenuPopup>
        </Menu>{" "}
        in{" "}
        <Part label={communities} disabled={disabled}>
          <CommunityPicker
            platform={platform}
            items={items}
            rule={rule}
            onRule={onRule}
          />
        </Part>{" "}
        <Part label={ageLabel(rule.age)} disabled={disabled} wide>
          <TimeChart
            items={items}
            age={rule.age}
            onAge={(age) => onRule({ ...rule, age })}
          />
        </Part>{" "}
        with{" "}
        <Part
          label={
            rule.maxScore === null
              ? "any score"
              : `fewer than ${count(rule.maxScore)} points`
          }
          disabled={disabled}
        >
          <ScorePicker rule={rule} onRule={onRule} />
        </Part>{" "}
        and {/* Each period stays on the line of the part before it. */}
        <span className="whitespace-nowrap">
          <Part
            label={rule.text ? `containing "${rule.text}"` : "any text"}
            disabled={disabled}
          >
            <TextPicker rule={rule} onRule={onRule} />
          </Part>
          {rule.keep.length > 0 ? "," : "."}
        </span>
        {rule.keep.length > 0 && (
          <>
            {" "}
            except{" "}
            {rule.keep.map((keep, index) => (
              <span key={`${keep.type}-${index}`} className="whitespace-nowrap">
                {index > 0 && "and "}
                <Button
                  variant="sentence"
                  size="sentence"
                  disabled={disabled}
                  aria-label={`Stop keeping ${keepLabel(keep, platform.community.prefix)}`}
                  onClick={() =>
                    onRule({
                      ...rule,
                      keep: rule.keep.filter((_, at) => at !== index),
                    })
                  }
                >
                  {keepLabel(keep, platform.community.prefix)}
                  <XIcon />
                </Button>
                {index === rule.keep.length - 1 ? "." : " "}
              </span>
            ))}
          </>
        )}{" "}
        <KeepAdder
          platform={platform}
          rule={rule}
          onRule={onRule}
          disabled={disabled}
        />
      </p>
    </div>
  );
};
