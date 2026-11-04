const fs = require('fs');
const path = require('path');
const { execSync } = require('child_process');

const rootDir = path.resolve(__dirname, '..');
const versionGoPath = path.join(rootDir, 'pkg', 'version', 'version.go');
const versionGo = fs.readFileSync(versionGoPath, 'utf8');

const majorMatch = versionGo.match(/Major\s*=\s*(\d+)/);
const minorMatch = versionGo.match(/Minor\s*=\s*(\d+)/);
const patchMatch = versionGo.match(/Patch\s*=\s*(\d+)/);

if (!majorMatch || !minorMatch || !patchMatch) {
  console.error('Failed to parse version from pkg/version/version.go');
  process.exit(1);
}

const major = majorMatch[1];
const minor = minorMatch[1];
const patch = patchMatch[1];
const versionString = `v${major}.${minor} p${patch}`;
const exeName = `Mortis ${versionString}.exe`;

console.log(`[Mortis Build] Version: ${versionString}`);
console.log(`[Mortis Build] Target Exe: ${exeName}`);

// Ensure build/ has its own go.mod so Go compiler never scans build/ as a package directory
const buildGoModPath = path.join(rootDir, 'build', 'go.mod');
if (!fs.existsSync(path.dirname(buildGoModPath))) {
  fs.mkdirSync(path.dirname(buildGoModPath), { recursive: true });
}
if (!fs.existsSync(buildGoModPath)) {
  fs.writeFileSync(buildGoModPath, 'module ignorebuild\n\ngo 1.22\n');
}

// Update wails.json
const wailsJsonPath = path.join(rootDir, 'wails.json');
const wailsJson = JSON.parse(fs.readFileSync(wailsJsonPath, 'utf8'));
wailsJson.outputfilename = `Mortis ${versionString}`;
wailsJson.info.productVersion = `${major}.${minor}.${patch}`;
fs.writeFileSync(wailsJsonPath, JSON.stringify(wailsJson, null, 2) + '\n');

// Also sync src/config/version.ts
const versionTsPath = path.join(rootDir, 'src', 'config', 'version.ts');
const versionTsContent = `/**
 * Centralized Mortis Version Configuration
 *
 * Rule of thumb:
 * - Leading number (v1, v2, v3) -> Malalaking update (major)
 * - Decimal (.0, .10, .25)       -> Mga bagong feature (minor)
 * - "p" suffix (p1, p2, p3...)  -> Maliliit na patch o bug fix (patch)
 */
export const APP_NAME = 'Mortis';

export const APP_VERSION = {
  major: ${major},
  minor: ${minor},
  patch: ${patch},
} as const;

export const APP_VERSION_STRING = \`v\${APP_VERSION.major}.\${APP_VERSION.minor} p\${APP_VERSION.patch}\`;
export const APP_WINDOW_TITLE = \`\${APP_NAME} \${APP_VERSION_STRING}\`;
`;
fs.writeFileSync(versionTsPath, versionTsContent);

// Compile frontend first to ensure dist/ is populated for go:embed
console.log(`[Mortis Build] Compiling frontend...`);
execSync(`npm run build:frontend`, { cwd: rootDir, stdio: 'inherit' });

// Run wails build
console.log(`[Mortis Build] Compiling binary with Wails...`);
execSync(`wails build -s -o "${exeName}"`, { cwd: rootDir, stdio: 'inherit' });

// Copy to project root
const builtExePath = path.join(rootDir, 'build', 'bin', exeName);
const rootExePath = path.join(rootDir, exeName);
if (fs.existsSync(builtExePath)) {
  fs.copyFileSync(builtExePath, rootExePath);
  console.log(`[Mortis Build] Successfully copied to: ${rootExePath}`);
}

// SoftEther runtime payload copy disabled while VPN feature is disabled
const enableVPNCopy = false;
if (enableVPNCopy) {
  const vpnSourceDir = path.join(rootDir, 'third_party', 'vpn');
  const vpnOutputDir = path.join(path.dirname(builtExePath), 'third_party', 'vpn');
  const vpnFiles = ['vpnclient_x64.exe', 'vpncmd_x64.exe', 'hamcore.se2'];
  const licenseNames = ['LICENSE.txt', 'License.txt', 'LICENSE', 'license.txt'];

  if (!fs.existsSync(vpnSourceDir)) {
    console.warn('[Mortis Build] SoftEther payload folder is missing; VPN support will require the payload before release.');
  } else {
    const licenseName = licenseNames.find((name) => fs.existsSync(path.join(vpnSourceDir, name)));
    const missing = vpnFiles.filter((name) => !fs.existsSync(path.join(vpnSourceDir, name)));
    if (missing.length > 0 || !licenseName) {
      const items = [...missing, ...(!licenseName ? ['SoftEther license file'] : [])];
      console.warn(`[Mortis Build] Incomplete third_party/vpn payload: ${items.join(', ')}. VPN feature will be unavailable until complete.`);
    } else {
      if (!fs.existsSync(vpnOutputDir)) {
        fs.mkdirSync(vpnOutputDir, { recursive: true });
      }
      for (const name of vpnFiles) {
        try {
          fs.copyFileSync(path.join(vpnSourceDir, name), path.join(vpnOutputDir, name));
        } catch (e) {
          // file may be in use
        }
      }
      try {
        fs.copyFileSync(path.join(vpnSourceDir, licenseName), path.join(vpnOutputDir, 'LICENSE.txt'));
      } catch (e) {}
      console.log(`[Mortis Build] Copied approved SoftEther files to: ${vpnOutputDir}`);
    }
  }
}
