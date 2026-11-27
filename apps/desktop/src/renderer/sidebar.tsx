import { Badge } from "@vanish/ui/components/badge";
import { Button } from "@vanish/ui/components/button";
import {
  Menu,
  MenuItem,
  MenuPopup,
  MenuRadioGroup,
  MenuRadioItem,
  MenuSeparator,
  MenuTrigger,
} from "@vanish/ui/components/menu";
import {
  ArrowBigUpIcon,
  BookmarkIcon,
  ChevronsUpDownIcon,
  CircleArrowDownIcon,
  CircleDotIcon,
  EyeOffIcon,
  FileArchiveIcon,
  FileTextIcon,
  HistoryIcon,
  MessageSquareIcon,
  PlusIcon,
  Settings2Icon,
  Trash2Icon,
  type LucideIcon,
} from "lucide-react";

import type { Account, AppState } from "../shared/api";
import { KINDS, PLATFORMS, type Kind } from "../shared/platforms";
import { vanish } from "./bridge";
import { ago, compact } from "./format";
import { Logo } from "./logo";
import { attempt } from "./notify";
import { TopRow } from "./top-row";

const ICONS: Record<Kind, LucideIcon> = {
  comments: MessageSquareIcon,
  posts: FileTextIcon,
  votes: ArrowBigUpIcon,
  saved: BookmarkIcon,
  hidden: EyeOffIcon,
};

const Avatar = ({ account }: { account: Account }) => (
  <span className="grid size-8 shrink-0 place-items-center rounded-lg bg-primary text-meta font-semibold text-primary-foreground uppercase">
    {account.name.replace(/^u\//, "").slice(0, 1)}
  </span>
);

export const Sidebar = ({
  state,
  account,
  kind,
  onAccount,
  onKind,
  onAddAccount,
  onRemoveAccount,
  onRecord,
  onReadExport,
  onHistory,
  onSettings,
}: {
  state: AppState;
  account: Account;
  kind: Kind;
  onAccount: (id: number) => void;
  onKind: (kind: Kind) => void;
  onAddAccount: () => void;
  onRemoveAccount: () => void;
  onRecord: () => void;
  onReadExport: () => void;
  onHistory: () => void;
  onSettings: () => void;
}) => {
  const platform = PLATFORMS[account.platform];
  const running = new Set(
    state.runs
      .filter((run) => run.accountId === account.id)
      .map((run) => run.kind),
  );
  return (
    <nav
      className="flex w-sidebar shrink-0 flex-col border-r bg-sidebar"
      aria-label="Accounts and lists"
    >
      <TopRow edge="left">
        <Logo className="size-5 text-primary" />
        <span className="font-semibold tracking-tight">Vanish</span>
        {state.demo && (
          <Badge variant="warning" className="ms-auto">
            Demo data
          </Badge>
        )}
      </TopRow>

      <div className="px-3 pt-1 pb-4">
        <Menu>
          <MenuTrigger
            render={
              <button
                type="button"
                className="flex w-full items-center gap-3 rounded-lg px-2 py-2 text-start hover:bg-hover data-popup-open:bg-hover"
              />
            }
          >
            <Avatar account={account} />
            <span className="min-w-0 flex-1">
              <span className="block truncate font-medium">{account.name}</span>
              <span className="block truncate text-meta text-muted-foreground">
                {account.scannedAt
                  ? `Updated ${ago(account.scannedAt)}`
                  : "Nothing found yet"}
              </span>
            </span>
            <ChevronsUpDownIcon className="size-4 text-muted-foreground" />
          </MenuTrigger>
          <MenuPopup align="start" className="w-64">
            <MenuRadioGroup
              value={String(account.id)}
              onValueChange={(value) => onAccount(Number(value))}
            >
              {state.accounts.map((option) => (
                <MenuRadioItem
                  key={option.id}
                  value={String(option.id)}
                  closeOnClick
                >
                  <span className="flex-1 truncate">{option.name}</span>
                  <span className="ms-4 text-meta text-muted-foreground">
                    {PLATFORMS[option.platform].name}
                  </span>
                </MenuRadioItem>
              ))}
            </MenuRadioGroup>
            <MenuSeparator />
            <MenuItem onClick={onAddAccount}>
              <PlusIcon />
              Add a {platform.name} account
            </MenuItem>
            <MenuItem onClick={onReadExport}>
              <FileArchiveIcon />
              Read {platform.name} data export
            </MenuItem>
            <MenuItem disabled={state.recording !== null} onClick={onRecord}>
              <CircleDotIcon />
              Record site traffic
            </MenuItem>
            <MenuSeparator />
            <MenuItem variant="destructive" onClick={onRemoveAccount}>
              <Trash2Icon />
              Remove this account
            </MenuItem>
          </MenuPopup>
        </Menu>
      </div>

      <ul className="flex flex-col gap-0.5 px-3">
        {KINDS.map((option) => {
          const Icon = ICONS[option];
          return (
            <li key={option}>
              <Button
                variant="ghost-muted"
                size="row"
                aria-current={option === kind ? "page" : undefined}
                onClick={() => onKind(option)}
              >
                <Icon />
                <span className="flex-1 text-start">
                  {platform.kinds[option].label}
                </span>
                {running.has(option) && (
                  <span
                    className="size-1.5 rounded-full bg-primary"
                    aria-label="Cleanup running"
                  />
                )}
                <span className="text-meta text-muted-foreground">
                  {compact(account.counts[option])}
                </span>
              </Button>
            </li>
          );
        })}
      </ul>

      <div className="mt-auto flex flex-col gap-0.5 px-3 pb-3">
        {state.update && (
          <Button
            variant="outline"
            className="mb-2 justify-start"
            disabled={!state.update.ready}
            onClick={() => void attempt(() => vanish.installUpdate())}
          >
            <CircleArrowDownIcon />
            {state.update.manual
              ? `Download Vanish ${state.update.version}`
              : state.update.ready
                ? `Restart to update to ${state.update.version}`
                : `Downloading ${state.update.version}`}
          </Button>
        )}
        <Button variant="ghost-muted" size="row" onClick={onHistory}>
          <HistoryIcon />
          Past cleanups
        </Button>
        <Button variant="ghost-muted" size="row" onClick={onSettings}>
          <Settings2Icon />
          Settings
        </Button>
      </div>
    </nav>
  );
};
