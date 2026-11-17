import type { SVGProps } from "react";

export function ContextCompactIcon({ all = false, ...props }: SVGProps<SVGSVGElement> & { all?: boolean }) {
  const rows = all ? [7, 17] : [12];
  return (
    <svg
      width="18"
      height="18"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      {...props}
    >
      {rows.map((y) => (
        <g key={y}>
          <path d={`M2 ${y}h7m-3 -3 3 3-3 3M22 ${y}h-7m3 -3-3 3 3 3`} />
        </g>
      ))}
    </svg>
  );
}
