import * as React from "react";
import { Clock, Flame, Heart, Star, type LucideProps } from "lucide-react";

import { cn } from "@/lib/utils";

export type StatPillKind = "star" | "heart" | "streak" | "time";
export type StatPillSize = "md" | "lg";

interface StatPillKindConfig {
  classes: string;
  icon: React.ComponentType<LucideProps>;
  iconClass: string;
}

const statPillKindConfig: Record<StatPillKind, StatPillKindConfig> = {
  star: { classes: "bg-sun-100 text-sun-600", icon: Star, iconClass: "text-sun-400" },
  heart: { classes: "bg-coral-100 text-coral-600", icon: Heart, iconClass: "text-coral-400" },
  streak: { classes: "bg-coral-100 text-coral-600", icon: Flame, iconClass: "text-coral-400" },
  time: { classes: "bg-sky-50 text-sky-500", icon: Clock, iconClass: "text-sky-400" },
};

interface StatPillSizeConfig {
  pill: string;
  icon: string;
}

const statPillSizeConfig: Record<StatPillSize, StatPillSizeConfig> = {
  md: { pill: "px-[13px] py-[7px] text-[length:var(--text-md)]", icon: "h-[18px] w-[18px]" },
  lg: { pill: "px-4 py-[9px] text-[length:var(--text-xl)]", icon: "h-[22px] w-[22px]" },
};

export interface StatPillProps extends React.HTMLAttributes<HTMLSpanElement> {
  /** Which gamification stat this pill represents. */
  kind: StatPillKind;
  /** The value shown next to the icon (e.g. a star count). */
  value: React.ReactNode;
  /** Size scale. Defaults to "md". */
  size?: StatPillSize;
}

export function StatPill({ kind, value, size = "md", className, ...rest }: StatPillProps) {
  const { classes, icon: Icon, iconClass } = statPillKindConfig[kind];
  const sizeConfig = statPillSizeConfig[size];

  return (
    <span
      className={cn(
        "inline-flex items-center gap-[6px] rounded-full font-display font-bold leading-none",
        classes,
        sizeConfig.pill,
        className,
      )}
      {...rest}
    >
      <Icon className={cn(sizeConfig.icon, iconClass)} strokeWidth={2} />
      {value}
    </span>
  );
}
