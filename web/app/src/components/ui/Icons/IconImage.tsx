import type { CSSProperties, ReactElement } from "react";
import "./Icons.css";

export function IconImage(name: string): ReactElement {
  const iconURL = `icons/${name}.svg`;
  return <span className="svg-icon" aria-hidden="true" style={iconMaskStyle(iconURL)}></span>;
}

function iconMaskStyle(iconURL: string): CSSProperties {
  return {
    WebkitMaskImage: `url("${iconURL}")`,
    maskImage: `url("${iconURL}")`,
  };
}

export function GlobeIcon(): ReactElement {
  return IconImage("globe");
}

export function SunIcon(): ReactElement {
  return IconImage("sun");
}

export function MoonIcon(): ReactElement {
  return IconImage("moon");
}

export function AddUserIcon(): ReactElement {
  return IconImage("add-user");
}

export function UsersIcon(): ReactElement {
  return IconImage("users");
}

export function WrenchIcon(): ReactElement {
  return IconImage("wrench");
}

export function SidebarToggleIcon(): ReactElement {
  return IconImage("sidebar-toggle");
}

export function ChevronIcon(): ReactElement {
  return IconImage("chevron");
}

export function RoomPlusIcon(): ReactElement {
  return IconImage("room-plus");
}

export function TrashIcon(): ReactElement {
  return IconImage("trash");
}

export function RoomsIcon(): ReactElement {
  return IconImage("rooms");
}

export function AgentIcon(): ReactElement {
  return IconImage("agent");
}

export function ComputerIcon(): ReactElement {
  return IconImage("computer");
}

export function HubIcon(): ReactElement {
  return IconImage("hub");
}

export function PlayIcon(): ReactElement {
  return IconImage("play");
}

export function StopIcon(): ReactElement {
  return IconImage("stop");
}
