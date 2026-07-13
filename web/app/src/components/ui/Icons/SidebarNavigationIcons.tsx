import type { SVGProps } from "react";

type SidebarNavigationIconProps = SVGProps<SVGSVGElement> & {
  size?: number | string;
};

function iconSize(size: number | string | undefined, fallback: number) {
  return size ?? fallback;
}

export function SidebarMessageIcon({ size, ...props }: SidebarNavigationIconProps) {
  const resolvedSize = iconSize(size, 24);
  return (
    <svg width={resolvedSize} height={resolvedSize} viewBox="0 0 24 24" fill="none" {...props}>
      <path
        d="M2 10C2 7.19974 2 5.79961 2.54497 4.73005C3.02433 3.78924 3.78924 3.02433 4.73005 2.54497C5.79961 2 7.19974 2 10 2H14C16.8003 2 18.2004 2 19.27 2.54497C20.2108 3.02433 20.9757 3.78924 21.455 4.73005C22 5.79961 22 7.19974 22 10V19.1708C22 20.1969 22 20.71 21.8373 21.0302C21.5642 21.5676 20.996 21.8893 20.3947 21.847C20.0363 21.8218 19.5964 21.5578 18.7165 21.0299C18.1917 20.715 17.9293 20.5576 17.6542 20.4347C17.1972 20.2306 16.7122 20.0962 16.2154 20.0362C15.9163 20 15.6103 20 14.9983 20H10C7.19974 20 5.79961 20 4.73005 19.455C3.78924 18.9757 3.02433 18.2108 2.54497 17.27C2 16.2004 2 14.8003 2 12V10Z"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <path d="M7 8H15M7 12H11" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

export function SidebarRobotIcon({ size, ...props }: SidebarNavigationIconProps) {
  const resolvedSize = iconSize(size, 24);
  return (
    <svg width={resolvedSize} height={resolvedSize} viewBox="0 0 24 24" fill="none" {...props}>
      <path
        d="M23 11V15.5M1 11V15.5M15 13V14M9 13V14M12 6.5C10.9333 6.5 9.86667 6.5 8.8 6.5C7.11984 6.5 6.27976 6.5 5.63803 6.82698C5.07354 7.1146 4.6146 7.57354 4.32698 8.13803C4 8.77976 4 9.61964 4 11.2994C4 12.7536 4 14.2999 4 15.7016C4 17.3807 4 18.2202 4.32698 18.862C4.6146 19.4265 5.07354 19.8854 5.63803 20.173C6.27976 20.5 7.11984 20.5 8.8 20.5C10.9333 20.5 13.0667 20.5 15.2 20.5C16.8802 20.5 17.7202 20.5 18.362 20.173C18.9265 19.8854 19.3854 19.4265 19.673 18.862C20 18.2202 20 17.3802 20 15.7V10.5002C20 9.57004 20 9.10498 19.8977 8.72342C19.6203 7.68824 18.8118 6.87968 17.7766 6.60225C17.395 6.5 16.93 6.5 15.9998 6.5C14.6666 6.5 13.3333 6.5 12 6.5ZM12 6.5V4.75V3M12 3L12.0625 2.9375M12 3L11.9375 2.9375M12 3V2.625M12.25 2.75H11.75M12.25 2.75L12.375 2.625M12.25 2.75L11.625 2.625M12.25 2.75L12.1875 2.8125M11.75 2.75L11.625 2.625M11.75 2.75L12.375 2.625M11.75 2.75L11.8125 2.8125M11.625 2.625L11.75 2.25M11.625 2.625H12M12.375 2.625C12.444 2.55596 12.444 2.44404 12.375 2.375L12.25 2.25C12.1119 2.11193 11.8881 2.11193 11.75 2.25L11.625 2.375C11.556 2.44404 11.556 2.55596 11.625 2.625M12.375 2.625L12.25 2.25M12.375 2.625H12M12.25 2.25H11.75M11.875 2.875H12.125M11.875 2.875L11.8125 2.8125M11.875 2.875L11.9375 2.9375M12.125 2.875L12.1875 2.8125M12.125 2.875L12.0625 2.9375M11.8125 2.8125H12.1875M12.0625 2.9375H11.9375"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

export function SidebarUserIcon({ size, ...props }: SidebarNavigationIconProps) {
  const resolvedSize = iconSize(size, 24);
  return (
    <svg width={resolvedSize} height={resolvedSize} viewBox="0 0 24 24" fill="none" {...props}>
      <path
        d="M4 18.8C4 16.149 6.14903 14 8.8 14H15.2C17.851 14 20 16.149 20 18.8C20 20.5673 18.5673 22 16.8 22H7.2C5.43269 22 4 20.5673 4 18.8Z"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <path
        d="M16 6C16 8.20914 14.2091 10 12 10C9.79086 10 8 8.20914 8 6C8 3.79086 9.79086 2 12 2C14.2091 2 16 3.79086 16 6Z"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

export function SidebarLaptopIcon({ size, ...props }: SidebarNavigationIconProps) {
  const resolvedSize = iconSize(size, 24);
  return (
    <svg width={resolvedSize} height={resolvedSize} viewBox="0 0 24 24" fill="none" {...props}>
      <path
        d="M21 17V9.4C21 7.15979 21 6.03968 20.564 5.18404C20.1805 4.43139 19.5686 3.81947 18.816 3.43597C17.9603 3 16.8402 3 14.6 3H9.4C7.15979 3 6.03968 3 5.18404 3.43597C4.43139 3.81947 3.81947 4.43139 3.43597 5.18404C3 6.03968 3 7.15979 3 9.4V17M4.55556 21H19.4444C21.4081 21 23 19.4081 23 17.4444C23 17.199 22.801 17 22.5556 17H1.44444C1.19898 17 1 17.199 1 17.4444C1 19.4081 2.59188 21 4.55556 21Z"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

export function SidebarAlertTriangleIcon({ size, ...props }: SidebarNavigationIconProps) {
  const resolvedSize = iconSize(size, 24);
  return (
    <svg width={resolvedSize} height={resolvedSize} viewBox="0 0 24 24" fill="none" {...props}>
      <path
        d="M12 13V9M12.5 16.5C12.5 16.7761 12.2761 17 12 17C11.7239 17 11.5 16.7761 11.5 16.5M12.5 16.5C12.5 16.2239 12.2761 16 12 16C11.7239 16 11.5 16.2239 11.5 16.5M12.5 16.5H11.5M19.3311 10.0912L18.98 9.46437C16.6988 5.39063 15.5581 3.35377 14.0576 2.67625C12.7495 2.08558 11.2505 2.08558 9.94239 2.67625C8.44189 3.35377 7.30124 5.39064 5.01995 9.46438L4.66894 10.0912C2.47606 14.007 1.37961 15.965 1.56302 17.5683C1.72303 18.967 2.46536 20.2335 3.60763 21.0566C4.91688 22 7.16092 22 11.649 22H12.351C16.8391 22 19.0831 22 20.3924 21.0566C21.5346 20.2335 22.277 18.967 22.437 17.5683C22.6204 15.965 21.5239 14.007 19.3311 10.0912Z"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

export function SidebarUsersIcon({ size, ...props }: SidebarNavigationIconProps) {
  const resolvedSize = iconSize(size, 24);
  return (
    <svg width={resolvedSize} height={resolvedSize} viewBox="0 0 24 24" fill="none" {...props}>
      <path
        d="M15 10C17.2091 10 19 8.20914 19 6C19 3.79086 17.2091 2 15 2M17 22H19.8C21.5673 22 23 20.5673 23 18.8C23 16.149 20.851 14 18.2 14H17M12 6C12 8.20914 10.2091 10 8 10C5.79086 10 4 8.20914 4 6C4 3.79086 5.79086 2 8 2C10.2091 2 12 3.79086 12 6ZM4.2 22H11.8C13.5673 22 15 20.5673 15 18.8C15 16.149 12.851 14 10.2 14H5.8C3.14903 14 1 16.149 1 18.8C1 20.5673 2.43269 22 4.2 22Z"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

export function SidebarListUnordered4Icon({ size, ...props }: SidebarNavigationIconProps) {
  const resolvedSize = iconSize(size, 24);
  return (
    <svg width={resolvedSize} height={resolvedSize} viewBox="0 0 24 24" fill="none" {...props}>
      <path
        d="M11 20L22 20M11 12L22 12M11 4L22 4M2 4L3 5L6 2M2 12L3 13L6 10M2 20L3 21L6 18"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

export function SidebarTimer2Icon({ size, ...props }: SidebarNavigationIconProps) {
  const resolvedSize = iconSize(size, 24);
  return (
    <svg width={resolvedSize} height={resolvedSize} viewBox="0 0 24 24" fill="none" {...props}>
      <path
        d="M8.66667 8.66667L12 12M4.92893 4.92893C3.11929 6.73858 2 9.23858 2 12C2 17.5228 6.47715 22 12 22C17.5228 22 22 17.5228 22 12C22 6.47715 17.5228 2 12 2V5.33333"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

export function SidebarGrid07Icon({ size, ...props }: SidebarNavigationIconProps) {
  const resolvedSize = iconSize(size, 24);
  return (
    <svg width={resolvedSize} height={resolvedSize} viewBox="0 0 24 24" fill="none" {...props}>
      <path
        d="M9 9L9 22M2 9H22M10 22H14C16.8003 22 18.2004 22 19.27 21.455C20.2108 20.9757 20.9757 20.2108 21.455 19.27C22 18.2004 22 16.8003 22 14V10C22 7.19974 22 5.79961 21.455 4.73005C20.9757 3.78924 20.2108 3.02433 19.27 2.54497C18.2004 2 16.8003 2 14 2H10C7.19974 2 5.79961 2 4.73005 2.54497C3.78924 3.02433 3.02433 3.78924 2.54497 4.73005C2 5.79961 2 7.19974 2 10V14C2 16.8003 2 18.2004 2.54497 19.27C3.02433 20.2108 3.78924 20.9757 4.73005 21.455C5.79961 22 7.19974 22 10 22Z"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

export function SidebarPuzzlePiece02Icon({ size, ...props }: SidebarNavigationIconProps) {
  const resolvedSize = iconSize(size, 24);
  return (
    <svg width={resolvedSize} height={resolvedSize} viewBox="0 0 24 24" fill="none" {...props}>
      <path
        d="M12 2L15.6 5.6C18 -0.7 24.7 6 18.4 8.4L22 12L18.4 15.6C16 9.3 9.3 16 15.6 18.4L12 22L8.4 18.4C6 24.7 -0.7 18 5.6 15.6L2 12L5.6 8.4C8 14.7 14.7 8 8.4 5.6L12 2Z"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

export function SidebarMcpIcon({ size, ...props }: SidebarNavigationIconProps) {
  const resolvedSize = iconSize(size, 16);
  return (
    <svg width={resolvedSize} height={resolvedSize} viewBox="0 0 16 16" fill="none" {...props}>
      <path
        d="M1.85339 7.54013L7.96123 1.43229C8.80459 0.588975 10.1719 0.588975 11.0152 1.43229C11.8585 2.2756 11.8585 3.64289 11.0152 4.48621L6.40246 9.09892"
        stroke="currentColor"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <path
        d="M6.46606 9.03509L11.0151 4.48601C11.8585 3.64269 13.2258 3.64269 14.0691 4.48601L14.1009 4.51781C14.9442 5.36113 14.9442 6.72842 14.1009 7.57173L8.57689 13.0957C8.29578 13.3769 8.29578 13.8326 8.57689 14.1137L9.71113 15.248"
        stroke="currentColor"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <path
        d="M9.48812 2.95898L4.97086 7.47624C4.12755 8.31953 4.12755 9.68681 4.97086 10.5302C5.81418 11.3734 7.18146 11.3734 8.02478 10.5302L12.542 6.0129"
        stroke="currentColor"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

export function SidebarBoxIcon({ size, ...props }: SidebarNavigationIconProps) {
  const resolvedSize = iconSize(size, 24);
  return (
    <svg width={resolvedSize} height={resolvedSize} viewBox="0 0 24 24" fill="none" {...props}>
      <path
        d="M12 12.5L21 7M12 12.5L3 7M12 12.5V22.5M2 9.71771V14.2823C2 15.2733 2 15.7688 2.14219 16.2141C2.26802 16.6081 2.47396 16.9718 2.74708 17.2824C3.05572 17.6334 3.48062 17.8884 4.33042 18.3983L9.53042 21.5183C10.4283 22.057 10.8773 22.3264 11.3565 22.4316C11.7805 22.5247 12.2195 22.5247 12.6435 22.4316C13.1227 22.3264 13.5717 22.057 14.4696 21.5183L19.6696 18.3983C20.5194 17.8884 20.9443 17.6334 21.2529 17.2824C21.526 16.9718 21.732 16.6081 21.8578 16.2141C22 15.7688 22 15.2733 22 14.2823V9.71771C22 8.72669 22 8.23117 21.8578 7.78593C21.732 7.39192 21.526 7.02818 21.2529 6.71757C20.9443 6.36657 20.5194 6.11163 19.6696 5.60175L14.4696 2.48175C13.5717 1.94301 13.1227 1.67364 12.6435 1.56839C12.2195 1.4753 11.7805 1.4753 11.3565 1.56839C10.8773 1.67364 10.4283 1.94301 9.53042 2.48175L4.33042 5.60175C3.48062 6.11163 3.05572 6.36657 2.74708 6.71757C2.47396 7.02818 2.26802 7.39192 2.14219 7.78593C2 8.23117 2 8.72669 2 9.71771Z"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

export function SidebarGearIcon({ size, ...props }: SidebarNavigationIconProps) {
  const resolvedSize = iconSize(size, 24);
  return (
    <svg width={resolvedSize} height={resolvedSize} viewBox="0 0 24 24" fill="none" {...props}>
      <path
        d="M12 3.99906C11.303 3.99906 10.6132 3.73734 10.1864 3.18636L9.73424 2.60274C9.12172 1.81211 8.01933 1.59548 7.15318 2.09555L5.84684 2.84977C4.98069 3.34984 4.6171 4.41285 4.99556 5.33862L5.27481 6.02173C5.5387 6.66724 5.42168 7.39223 5.0722 7.99571C4.72303 8.59864 4.15039 9.06946 3.46003 9.1636L2.72977 9.26318C1.7388 9.39832 1 10.2447 1 11.2448V12.7533C1 13.7534 1.7388 14.5998 2.72977 14.7349L3.46006 14.8345C4.1504 14.9287 4.72303 15.3995 5.07219 16.0024C5.42166 16.6059 5.53867 17.3308 5.27479 17.9763L4.99554 18.6594C4.61708 19.5852 4.98067 20.6482 5.84682 21.1483L7.15316 21.9025C8.01931 22.4026 9.1217 22.1859 9.73422 21.3953L10.1863 20.8118C10.6131 20.2608 11.303 19.9991 12 19.9991C12.6971 19.9991 13.387 20.2608 13.8139 20.8119L14.2658 21.3951C14.8783 22.1858 15.9807 22.4024 16.8468 21.9023L18.1532 21.1481C19.0193 20.648 19.3829 19.585 19.0045 18.6592L18.7252 17.9762C18.4614 17.3307 18.5784 16.6058 18.9278 16.0024C19.277 15.3995 19.8496 14.9287 20.5399 14.8345L21.2702 14.7349C22.2612 14.5998 23 13.7534 23 12.7533V11.2448C23 10.2447 22.2612 9.39832 21.2702 9.26319L20.5399 9.1636C19.8496 9.06946 19.277 8.59866 18.9278 7.99573C18.5784 7.39228 18.4613 6.66732 18.7252 6.02184L19.0044 5.3388C19.3829 4.41303 19.0193 3.35001 18.1532 2.84994L16.8468 2.09572C15.9807 1.59565 14.8783 1.81228 14.2658 2.60292L13.8138 3.18628C13.3869 3.73731 12.697 3.99906 12 3.99906Z"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <path
        d="M15 11.999C15 13.6559 13.6569 14.999 12 14.999C10.3431 14.999 9 13.6559 9 11.999C9 10.3422 10.3431 8.99902 12 8.99902C13.6569 8.99902 15 10.3422 15 11.999Z"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

export function SidebarLayoutRightIcon({ size, ...props }: SidebarNavigationIconProps) {
  const resolvedSize = iconSize(size, 20);
  return (
    <svg width={resolvedSize} height={resolvedSize} viewBox="0 0 20 20" fill="none" {...props}>
      <path
        d="M12.5 2.5V17.5M6.5 2.5H13.5C14.9001 2.5 15.6002 2.5 16.135 2.77248C16.6054 3.01217 16.9878 3.39462 17.2275 3.86502C17.5 4.3998 17.5 5.09987 17.5 6.5V13.5C17.5 14.9001 17.5 15.6002 17.2275 16.135C16.9878 16.6054 16.6054 16.9878 16.135 17.2275C15.6002 17.5 14.9001 17.5 13.5 17.5H6.5C5.09987 17.5 4.3998 17.5 3.86502 17.2275C3.39462 16.9878 3.01217 16.6054 2.77248 16.135C2.5 15.6002 2.5 14.9001 2.5 13.5V6.5C2.5 5.09987 2.5 4.3998 2.77248 3.86502C3.01217 3.39462 3.39462 3.01217 3.86502 2.77248C4.3998 2.5 5.09987 2.5 6.5 2.5Z"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

export function SidebarLayoutLeftIcon({ size, ...props }: SidebarNavigationIconProps) {
  const resolvedSize = iconSize(size, 20);
  return (
    <svg width={resolvedSize} height={resolvedSize} viewBox="0 0 20 20" fill="none" {...props}>
      <path
        d="M7.5 2.5V17.5M6.5 2.5H13.5C14.9001 2.5 15.6002 2.5 16.135 2.77248C16.6054 3.01217 16.9878 3.39462 17.2275 3.86502C17.5 4.3998 17.5 5.09987 17.5 6.5V13.5C17.5 14.9001 17.5 15.6002 17.2275 16.135C16.9878 16.6054 16.6054 16.9878 16.135 17.2275C15.6002 17.5 14.9001 17.5 13.5 17.5H6.5C5.09987 17.5 4.3998 17.5 3.86502 17.2275C3.39462 16.9878 3.01217 16.6054 2.77248 16.135C2.5 15.6002 2.5 14.9001 2.5 13.5V6.5C2.5 5.09987 2.5 4.3998 2.77248 3.86502C3.01217 3.39462 3.39462 3.01217 3.86502 2.77248C4.3998 2.5 5.09987 2.5 6.5 2.5Z"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}
