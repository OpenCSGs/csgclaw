export enum DesktopPlatform {
  MacOS = "darwin",
  Windows = "win32",
  Linux = "linux",
}

export enum DesktopArchitecture {
  X64 = "x64",
  ARM64 = "arm64",
}

export enum GoOperatingSystem {
  MacOS = "darwin",
  Windows = "windows",
  Linux = "linux",
}

export enum GoArchitecture {
  AMD64 = "amd64",
  ARM64 = "arm64",
}

export enum CSGClawExecutableName {
  Posix = "csgclaw",
  Windows = "csgclaw.exe",
}

export const CSGCLAW_EXECUTABLE_BY_PLATFORM = {
  [DesktopPlatform.MacOS]: CSGClawExecutableName.Posix,
  [DesktopPlatform.Windows]: CSGClawExecutableName.Windows,
  [DesktopPlatform.Linux]: CSGClawExecutableName.Posix,
} as const satisfies Record<DesktopPlatform, string>;

export const GO_OS_BY_DESKTOP_PLATFORM = {
  [DesktopPlatform.MacOS]: GoOperatingSystem.MacOS,
  [DesktopPlatform.Windows]: GoOperatingSystem.Windows,
  [DesktopPlatform.Linux]: GoOperatingSystem.Linux,
} as const satisfies Record<DesktopPlatform, GoOperatingSystem>;

export const GO_ARCH_BY_DESKTOP_ARCH = {
  [DesktopArchitecture.X64]: GoArchitecture.AMD64,
  [DesktopArchitecture.ARM64]: GoArchitecture.ARM64,
} as const satisfies Record<DesktopArchitecture, GoArchitecture>;

export const DESKTOP_ARCH_BY_GO_ARCH = {
  [GoArchitecture.AMD64]: DesktopArchitecture.X64,
  [GoArchitecture.ARM64]: DesktopArchitecture.ARM64,
} as const satisfies Record<GoArchitecture, DesktopArchitecture>;

export function csgclawExecutableForPlatform(platform: NodeJS.Platform): string {
  return (
    CSGCLAW_EXECUTABLE_BY_PLATFORM[platform as DesktopPlatform] ??
    CSGCLAW_EXECUTABLE_BY_PLATFORM[DesktopPlatform.Linux]
  );
}

export function goOSForDesktopPlatform(platform: NodeJS.Platform): string {
  return GO_OS_BY_DESKTOP_PLATFORM[platform as DesktopPlatform] ?? platform;
}

export function goArchForDesktopArch(arch: string): string {
  return GO_ARCH_BY_DESKTOP_ARCH[arch as DesktopArchitecture] ?? arch;
}

export function desktopArchForGoArch(arch: string): string {
  return DESKTOP_ARCH_BY_GO_ARCH[arch as GoArchitecture] ?? arch;
}
