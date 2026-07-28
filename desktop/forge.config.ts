import fs from "node:fs";
import path from "node:path";
import type { ForgeConfig } from "@electron-forge/shared-types";
import { MakerDeb } from "@electron-forge/maker-deb";
import { MakerDMG } from "@electron-forge/maker-dmg";
import { MakerSquirrel } from "@electron-forge/maker-squirrel";
import { MakerZIP } from "@electron-forge/maker-zip";
import { FusesPlugin } from "@electron-forge/plugin-fuses";
import { FuseV1Options, FuseVersion } from "@electron/fuses";

const targetGoOS = process.env.CSGCLAW_DESKTOP_GOOS || electronPlatformToGoOS(process.platform);
const targetGoArch = process.env.CSGCLAW_DESKTOP_GOARCH || electronArchToGoArch(process.arch);
const targetElectronArch = process.env.CSGCLAW_DESKTOP_ARCH || goArchToElectronArch(targetGoArch);
const backendResources = path.resolve(
  __dirname,
  "..",
  "dist",
  "desktop-input",
  `${targetGoOS}-${targetGoArch}`,
  "backend",
);
const entitlements = path.resolve(__dirname, "resources", "entitlements", "macos.plist");
const iconDirectory = path.resolve(__dirname, "resources", "icons");
const macIcon = path.join(iconDirectory, "csgclaw.icns");
const windowsIcon = path.join(iconDirectory, "csgclaw.ico");
const linuxIcon = path.join(iconDirectory, "csgclaw.png");
const appIcon = targetGoOS === "darwin" ? macIcon : targetGoOS === "windows" ? windowsIcon : linuxIcon;
const adHocEntitlements = path.resolve(
  __dirname,
  "resources",
  "entitlements",
  "macos-adhoc.plist",
);
const updateConfig = path.resolve(__dirname, ".forge-generated", "desktop-update.json");
const requestedVersion = (process.env.CSGCLAW_DESKTOP_VERSION || "").trim().replace(/^v/, "");
const versionParts = /^(\d+)\.(\d+)\.(\d+)/.exec(requestedVersion);
const desktopVersion = versionParts
  ? `${versionParts[1]}.${versionParts[2]}.${versionParts[3]}`
  : "0.0.0-development";
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
const enableCookieEncryption = targetGoOS !== "darwin" || !usesAdHocMacSignature;
const windowsSign =
  process.env.CSGCLAW_WINDOWS_SIGN_TOOL && process.env.CSGCLAW_WINDOWS_SIGN_PARAMS
    ? {
        signToolPath: process.env.CSGCLAW_WINDOWS_SIGN_TOOL,
        signWithParams: process.env.CSGCLAW_WINDOWS_SIGN_PARAMS,
      }
    : undefined;

const config: ForgeConfig = {
  packagerConfig: {
    appBundleId: "com.opencsg.csgclaw.desktop",
    appCategoryType: "public.app-category.developer-tools",
    appVersion: desktopVersion,
    asar: true,
    executableName: "CSGClaw",
    extraResource: [backendResources, updateConfig],
    icon: appIcon,
    name: "CSGClaw",
    osxSign: {
      hardenedRuntime: true,
      entitlements,
      entitlementsInherit: entitlements,
      continueOnError: false,
      ignore: (filePath) =>
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
    new MakerSquirrel({
      name: "csgclaw_desktop",
      setupIcon: windowsIcon,
      setupExe: `CSGClaw-Desktop-${desktopVersion}-${targetElectronArch}-Setup.exe`,
      ...(updateBaseURL ? { remoteReleases: `${updateBaseURL}/win32/${targetElectronArch}` } : {}),
      ...(windowsSign ? { windowsSign } : {}),
      ...(process.env.CSGCLAW_WINDOWS_CERTIFICATE_FILE && process.env.CSGCLAW_WINDOWS_CERTIFICATE_PASSWORD
        ? {
            certificateFile: process.env.CSGCLAW_WINDOWS_CERTIFICATE_FILE,
            certificatePassword: process.env.CSGCLAW_WINDOWS_CERTIFICATE_PASSWORD,
          }
        : {}),
    }),
    new MakerZIP(
      updateBaseURL ? { macUpdateManifestBaseUrl: `${updateBaseURL}/darwin/${targetElectronArch}` } : {},
      ["darwin"],
    ),
    new MakerDMG({
      format: "ULFO",
      name: `CSGClaw-Desktop-${desktopVersion}-${targetElectronArch}`,
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

function electronArchToGoArch(arch: string): string {
  return arch === "x64" ? "amd64" : arch;
}

function electronPlatformToGoOS(platform: NodeJS.Platform): string {
  return platform === "win32" ? "windows" : platform;
}

function goArchToElectronArch(arch: string): string {
  return arch === "amd64" ? "x64" : arch;
}

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

export default config;
