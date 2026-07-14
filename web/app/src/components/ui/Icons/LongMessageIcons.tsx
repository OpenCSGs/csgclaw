import type { SVGProps } from "react";

type LongMessageIconProps = SVGProps<SVGSVGElement> & {
  size?: number | string;
};

function iconSize(size: number | string | undefined) {
  return size ?? 24;
}

export function LongMessageExpandIcon({ size, ...props }: LongMessageIconProps) {
  const resolvedSize = iconSize(size);
  return (
    <svg width={resolvedSize} height={resolvedSize} viewBox="0 0 24 24" fill="none" {...props}>
      <path
        d="M6 9L12 15L18 9"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

export function LongMessageCollapseIcon({ size, ...props }: LongMessageIconProps) {
  const resolvedSize = iconSize(size);
  return (
    <svg width={resolvedSize} height={resolvedSize} viewBox="0 0 24 24" fill="none" {...props}>
      <path
        d="M6 15L12 9L18 15"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}
