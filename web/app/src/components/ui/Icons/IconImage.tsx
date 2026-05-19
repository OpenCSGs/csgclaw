import type { CSSProperties } from "react";
import "./Icons.css";

export function IconImage(name: string) {
  return (
    <span
      className="svg-icon"
      aria-hidden="true"
      style={
        {
          WebkitMaskImage: `url("/icons/${name}.svg")`,
          maskImage: `url("/icons/${name}.svg")`,
        } as CSSProperties
      }
    ></span>
  );
}

export function GlobeIcon() {
  return IconImage("globe");
}

export function SunIcon() {
  return IconImage("sun");
}

export function MoonIcon() {
  return IconImage("moon");
}

export function AddUserIcon() {
  return IconImage("add-user");
}

export function UsersIcon() {
  return IconImage("users");
}

export function WrenchIcon() {
  return IconImage("wrench");
}

export function SidebarToggleIcon() {
  return IconImage("sidebar-toggle");
}

export function ChevronIcon() {
  return IconImage("chevron");
}

export function RoomPlusIcon() {
  return IconImage("room-plus");
}

export function TrashIcon() {
  return IconImage("trash");
}

export function RoomsIcon() {
  return IconImage("rooms");
}

export function AgentIcon() {
  return IconImage("agent");
}

export function ComputerIcon() {
  return IconImage("computer");
}

export function HubIcon() {
  return IconImage("hub");
}

export function PlayIcon() {
  return IconImage("play");
}

export function StopIcon() {
  return IconImage("stop");
}
