import fs from "node:fs";
import path from "node:path";
import type { ForgeConfig } from "@electron-forge/shared-types";
import { MakerDeb } from "@electron-forge/maker-deb";
import { MakerDMG } from "@electron-forge/maker-dmg";
import { MakerMSIX } from "@electron-forge/maker-msix";
import { MakerSquirrel } from "@electron-forge/maker-squirrel";
import { MakerZIP } from "@electron-forge/maker-zip";
import { FusesPlugin } from "@electron-forge/plugin-fuses";
import { FuseV1Options, FuseVersion } from "@electron/fuses";
import {
  desktopArchForGoArch,
  DesktopPlatform,
  GoOperatingSystem,
  goArchForDesktopArch,
  goOSForDesktopPlatform,
} from "./src/shared/desktopEnvironment";
import {
  normalizeDesktopReleaseVersion,
  numericDesktopAppVersion,
} from "./src/shared/releaseVersion";

const targetGoOS = process.env.CSGCLAW_DESKTOP_GOOS || goOSForDesktopPlatform(process.platform);
const targetGoArch = process.env.CSGCLAW_DESKTOP_GOARCH || goArchForDesktopArch(process.arch);
const targetElectronArch = process.env.CSGCLAW_DESKTOP_ARCH || desktopArchForGoArch(targetGoArch);
const isMacTarget = targetGoOS === GoOperatingSystem.MacOS;
const isWindowsTarget = targetGoOS === GoOperatingSystem.Windows;
const backendResources = path.resolve(
  __dirname,
  "out",
  "input",
  `${targetGoOS}-${targetGoArch}`,
  "backend",
);
const entitlements = path.resolve(__dirname, "resources", "entitlements", "macos.plist");
const iconDirectory = path.resolve(__dirname, "resources", "icons");
const macIcon = path.join(iconDirectory, "csgclaw-theme.icns");
const macDockLightIcon = path.join(iconDirectory, "csgclaw-dock-light.png");
const macDockDarkIcon = path.join(iconDirectory, "csgclaw-dock-dark.png");
const markLightIcon = path.join(iconDirectory, "csgclaw-mark-light.svg");
const markDarkIcon = path.join(iconDirectory, "csgclaw-mark-dark.svg");
const windowsIcon = path.join(iconDirectory, "csgclaw.ico");
const linuxIcon = path.join(iconDirectory, "csgclaw.png");
const windowsIconURL =
  "https://raw.githubusercontent.com/OpenCSGs/csgclaw/main/desktop/resources/icons/csgclaw.ico";
const msixAssets = path.resolve(__dirname, "resources", "msix");
const appIcon = isMacTarget ? macIcon : isWindowsTarget ? windowsIcon : linuxIcon;
const adHocEntitlements = path.resolve(
  __dirname,
  "resources",
  "entitlements",
  "macos-adhoc.plist",
);
const updateConfig = path.resolve(__dirname, ".forge-generated", "desktop-update.json");
const desktopVersion = normalizeDesktopReleaseVersion(process.env.CSGCLAW_DESKTOP_VERSION);
const desktopAppVersion = numericDesktopAppVersion(desktopVersion);
const updateBaseURL = normalizeHTTPSBaseURL(process.env.CSGCLAW_DESKTOP_UPDATE_BASE_URL);
fs.mkdirSync(path.dirname(updateConfig), { recursive: true });
fs.writeFileSync(updateConfig, `${JSON.stringify({ base_url: updateBaseURL || "" }, null, 2)}\n`, { mode: 0o600 });
const requestedMacSignIdentity = process.env.CSGCLAW_MACOS_SIGN_IDENTITY?.trim();
const hasAppleNotarizationCredentials = Boolean(
  process.env.APPLE_ID && process.env.APPLE_PASSWORD && process.env.APPLE_TEAM_ID,
);
const macSignIdentity =
  requestedMacSignIdentity || (!hasAppleNotarizationCredentials ? "-" : undefined);
const usesAdHocMacSignature = macSignIdentity === "-";
const skipMacSigning = process.env.CSGCLAW_MACOS_SKIP_SIGN === "1";
const enableCookieEncryption = !isMacTarget || !usesAdHocMacSignature;
const windowsSign =
  process.env.CSGCLAW_WINDOWS_SIGN_TOOL && process.env.CSGCLAW_WINDOWS_SIGN_PARAMS
    ? {
        signToolPath: process.env.CSGCLAW_WINDOWS_SIGN_TOOL,
        signWithParams: process.env.CSGCLAW_WINDOWS_SIGN_PARAMS,
      }
    : undefined;
const windowsPackageChannel = resolveWindowsPackageChannel(
  process.env.CSGCLAW_DESKTOP_WINDOWS_CHANNEL,
);
const makeSquirrel = windowsPackageChannel !== "store";
const makeMSIX = isWindowsTarget && windowsPackageChannel !== "website";
const msixIdentity = makeMSIX
  ? requireEnvironmentVariables([
      "CSGCLAW_MSIX_IDENTITY_NAME",
      "CSGCLAW_MSIX_PUBLISHER",
      "CSGCLAW_MSIX_PUBLISHER_DISPLAY_NAME",
    ])
  : undefined;

const config: ForgeConfig = {
  packagerConfig: {
    appBundleId: "com.opencsg.csgclaw.desktop",
    appCategoryType: "public.app-category.developer-tools",
    appVersion: desktopAppVersion,
    asar: true,
    executableName: "CSGClaw",
    extraResource: [
      backendResources,
      updateConfig,
      markLightIcon,
      markDarkIcon,
      ...(isMacTarget ? [macDockLightIcon, macDockDarkIcon] : []),
      ...(isWindowsTarget ? [windowsIcon] : []),
    ],
    icon: appIcon,
    name: "CSGClaw",
    ...(isMacTarget && !skipMacSigning
      ? {
          osxSign: {
            hardenedRuntime: true,
            entitlements,
            entitlementsInherit: entitlements,
            continueOnError: false,
            ignore: (filePath: string) =>
              filePath.endsWith(path.join("sandbox-tools", "csgclaw-cli")),
            ...(macSignIdentity
              ? {
                  identity: macSignIdentity,
                  identityValidation: !usesAdHocMacSignature,
                  ...(usesAdHocMacSignature
                    ? {
                        timestamp: "none",
                        optionsForFile: (filePath: string) =>
                          path.extname(filePath) === ".app"
                            ? { entitlements: adHocEntitlements }
                            : {},
                      }
                    : {}),
                }
              : {}),
          },
        }
      : {}),
    ...(windowsSign ? { windowsSign } : {}),
    ...(hasAppleNotarizationCredentials
      ? {
          osxNotarize: {
            appleId: process.env.APPLE_ID!,
            appleIdPassword: process.env.APPLE_PASSWORD!,
            teamId: process.env.APPLE_TEAM_ID!,
          },
        }
      : {}),
  },
  rebuildConfig: {},
  makers: [
    ...(makeSquirrel
      ? [
          new MakerSquirrel({
            name: "csgclaw_desktop",
            iconUrl: windowsIconURL,
            setupIcon: windowsIcon,
            setupExe: `CSGClaw-Desktop-${desktopVersion}-${targetElectronArch}-Setup.exe`,
            ...(updateBaseURL
              ? { remoteReleases: `${updateBaseURL}/${DesktopPlatform.Windows}/${targetElectronArch}` }
              : {}),
            ...(windowsSign ? { windowsSign } : {}),
            ...(process.env.CSGCLAW_WINDOWS_CERTIFICATE_FILE &&
            process.env.CSGCLAW_WINDOWS_CERTIFICATE_PASSWORD
              ? {
                  certificateFile: process.env.CSGCLAW_WINDOWS_CERTIFICATE_FILE,
                  certificatePassword: process.env.CSGCLAW_WINDOWS_CERTIFICATE_PASSWORD,
                }
              : {}),
          }),
        ]
      : []),
    ...(makeMSIX && msixIdentity
      ? [
          new MakerMSIX({
            packageAssets: msixAssets,
            createPri: true,
            logLevel: "warn",
            ...(process.env.CSGCLAW_MSIX_WINDOWS_KIT_VERSION
              ? { windowsKitVersion: process.env.CSGCLAW_MSIX_WINDOWS_KIT_VERSION }
              : {}),
            ...(windowsSign ? { windowsSignOptions: windowsSign } : {}),
            manifestVariables: {
              packageIdentity: msixIdentity.CSGCLAW_MSIX_IDENTITY_NAME,
              publisher: msixIdentity.CSGCLAW_MSIX_PUBLISHER,
              publisherDisplayName: msixIdentity.CSGCLAW_MSIX_PUBLISHER_DISPLAY_NAME,
              packageDisplayName: "CSGClaw",
              appDisplayName: "CSGClaw",
              packageDescription: "CSGClaw Desktop",
              packageBackgroundColor: "transparent",
            },
          }),
        ]
      : []),
    new MakerZIP(
      updateBaseURL
        ? { macUpdateManifestBaseUrl: `${updateBaseURL}/${DesktopPlatform.MacOS}/${targetElectronArch}` }
        : {},
      [DesktopPlatform.MacOS],
    ),
    new MakerDMG({
      format: "ULFO",
    }),
    new MakerDeb({
      options: {
        bin: "CSGClaw",
        categories: ["Development"],
        genericName: "CSGClaw Desktop",
        homepage: "https://github.com/OpenCSGs/csgclaw",
        icon: linuxIcon,
        maintainer: "OpenCSG",
      },
    }),
  ],
  plugins: [
    new FusesPlugin({
      version: FuseVersion.V1,
      [FuseV1Options.RunAsNode]: false,
      [FuseV1Options.EnableCookieEncryption]: enableCookieEncryption,
      [FuseV1Options.EnableNodeOptionsEnvironmentVariable]: false,
      [FuseV1Options.EnableNodeCliInspectArguments]: false,
      [FuseV1Options.EnableEmbeddedAsarIntegrityValidation]: true,
      [FuseV1Options.OnlyLoadAppFromAsar]: true,
      [FuseV1Options.GrantFileProtocolExtraPrivileges]: false,
    }),
  ],
  hooks: {
    readPackageJson: async (_forgeConfig, packageJSON) => ({
      ...packageJSON,
      version: desktopVersion,
    }),
  },
};

function normalizeHTTPSBaseURL(rawURL: string | undefined): string {
  const value = rawURL?.trim();
  if (!value) {
    return "";
  }
  const parsed = new URL(value);
  if (parsed.protocol !== "https:" || parsed.username || parsed.password || parsed.search || parsed.hash) {
    throw new Error("CSGCLAW_DESKTOP_UPDATE_BASE_URL must be an HTTPS URL without credentials, query, or fragment.");
  }
  return parsed.toString().replace(/\/+$/, "");
}

function resolveWindowsPackageChannel(
  rawChannel: string | undefined,
): "website" | "store" | "all" {
  const channel = rawChannel?.trim().toLowerCase() || "website";
  if (channel === "website" || channel === "store" || channel === "all") {
    return channel;
  }
  throw new Error(
    "CSGCLAW_DESKTOP_WINDOWS_CHANNEL must be website, store, or all.",
  );
}

function requireEnvironmentVariables<const Name extends string>(
  names: readonly Name[],
): Record<Name, string> {
  const values = {} as Record<Name, string>;
  const missing: string[] = [];
  for (const name of names) {
    const value = process.env[name]?.trim();
    if (!value) {
      missing.push(name);
    } else {
      values[name] = value;
    }
  }
  if (missing.length > 0) {
    throw new Error(
      `Microsoft Store MSIX packaging requires: ${missing.join(", ")}.`,
    );
  }
  return values;
}

export default config;
