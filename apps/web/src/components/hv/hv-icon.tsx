import * as React from "react";
import {
  Check,
  FileText,
  Home,
  Plus,
  Send,
  Users,
  Wallet,
  X,
  type LucideProps,
} from "lucide-react";

export type HvIconName = "home" | "check" | "users" | "file" | "send" | "wallet" | "x" | "plus";

const iconRegistry: Record<HvIconName, React.ComponentType<LucideProps>> = {
  home: Home,
  check: Check,
  users: Users,
  file: FileText,
  send: Send,
  wallet: Wallet,
  x: X,
  plus: Plus,
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
