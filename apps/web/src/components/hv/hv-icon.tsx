import * as React from "react";
import {
  AlertTriangle,
  ArrowDown,
  ArrowUp,
  Check,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  Clock,
  FileText,
  Home,
  Info,
  Plus,
  Send,
  Table,
  Trash2,
  Users,
  Wallet,
  X,
  type LucideProps,
} from "lucide-react";

export type HvIconName =
  | "home"
  | "check"
  | "chevron-down"
  | "chevron-left"
  | "chevron-right"
  | "clock"
  | "users"
  | "file"
  | "send"
  | "wallet"
  | "x"
  | "plus"
  | "arrow-up"
  | "arrow-down"
  | "trash"
  | "table"
  | "info"
  | "alert";

const iconRegistry: Record<HvIconName, React.ComponentType<LucideProps>> = {
  home: Home,
  check: Check,
  "chevron-down": ChevronDown,
  "chevron-left": ChevronLeft,
  "chevron-right": ChevronRight,
  clock: Clock,
  users: Users,
  file: FileText,
  send: Send,
  wallet: Wallet,
  x: X,
  plus: Plus,
  "arrow-up": ArrowUp,
  "arrow-down": ArrowDown,
  trash: Trash2,
  table: Table,
  info: Info,
  alert: AlertTriangle,
};

const DEFAULT_STROKE_WIDTH = 2;
const DEFAULT_SIZE = 20;

export interface HvIconProps extends LucideProps {
  /** Which icon from the prototype's set to render. */
  name: HvIconName;
}

/** Generic icon wrapper applying the DS defaults (stroke 2, size 20, round caps). */
export function HvIcon({
  name,
  strokeWidth = DEFAULT_STROKE_WIDTH,
  size = DEFAULT_SIZE,
  ...rest
}: HvIconProps) {
  const Icon = iconRegistry[name];
  return <Icon strokeWidth={strokeWidth} size={size} {...rest} />;
}

export function HvHomeIcon({
  strokeWidth = DEFAULT_STROKE_WIDTH,
  size = DEFAULT_SIZE,
  ...rest
}: LucideProps) {
  return <Home strokeWidth={strokeWidth} size={size} {...rest} />;
}

export function HvCheckIcon({
  strokeWidth = DEFAULT_STROKE_WIDTH,
  size = DEFAULT_SIZE,
  ...rest
}: LucideProps) {
  return <Check strokeWidth={strokeWidth} size={size} {...rest} />;
}

export function HvClockIcon({
  strokeWidth = DEFAULT_STROKE_WIDTH,
  size = DEFAULT_SIZE,
  ...rest
}: LucideProps) {
  return <Clock strokeWidth={strokeWidth} size={size} {...rest} />;
}

export function HvUsersIcon({
  strokeWidth = DEFAULT_STROKE_WIDTH,
  size = DEFAULT_SIZE,
  ...rest
}: LucideProps) {
  return <Users strokeWidth={strokeWidth} size={size} {...rest} />;
}

export function HvFileIcon({
  strokeWidth = DEFAULT_STROKE_WIDTH,
  size = DEFAULT_SIZE,
  ...rest
}: LucideProps) {
  return <FileText strokeWidth={strokeWidth} size={size} {...rest} />;
}

export function HvSendIcon({
  strokeWidth = DEFAULT_STROKE_WIDTH,
  size = DEFAULT_SIZE,
  ...rest
}: LucideProps) {
  return <Send strokeWidth={strokeWidth} size={size} {...rest} />;
}

export function HvWalletIcon({
  strokeWidth = DEFAULT_STROKE_WIDTH,
  size = DEFAULT_SIZE,
  ...rest
}: LucideProps) {
  return <Wallet strokeWidth={strokeWidth} size={size} {...rest} />;
}

export function HvXIcon({
  strokeWidth = DEFAULT_STROKE_WIDTH,
  size = DEFAULT_SIZE,
  ...rest
}: LucideProps) {
  return <X strokeWidth={strokeWidth} size={size} {...rest} />;
}

export function HvPlusIcon({
  strokeWidth = DEFAULT_STROKE_WIDTH,
  size = DEFAULT_SIZE,
  ...rest
}: LucideProps) {
  return <Plus strokeWidth={strokeWidth} size={size} {...rest} />;
}
