#!/usr/bin/env node

const { spawnSync } = require("child_process");
const path = require("path");
const fs = require("fs");
const os = require("os");
const https = require("https");
const { version: packageVersion } = require("../package.json");

const REPO = "MabudAlam/QuickCrawl";
const BINARY_NAME = "quickcrawl-mcp";

const PLATFORMS = {
  "darwin-arm64": { binary: "quickcrawl-mcp", ext: ".tar.gz" },
  "darwin-x64": { binary: "quickcrawl-mcp", ext: ".tar.gz" },
  "linux-arm64": { binary: "quickcrawl-mcp", ext: ".tar.gz" },
  "linux-x64": { binary: "quickcrawl-mcp", ext: ".tar.gz" },
  "win32-x64": { binary: "quickcrawl-mcp.exe", ext: ".zip" },
  "win32-arm64": { binary: "quickcrawl-mcp.exe", ext: ".zip" },
};

function getArchiveName(version) {
  const p = PLATFORMS[key];
  const prefix = `${BINARY_NAME}_${version}_${process.platform === "win32" ? "windows" : process.platform}_${process.arch === "x64" ? "amd64" : "arm64"}`;
  return p.ext === ".zip" ? `${prefix}.zip` : `${prefix}.tar.gz`;
}

const key = `${process.platform}-${process.arch}`;
const platform = PLATFORMS[key];

if (!platform) {
  console.error(`quickcrawl-mcp: unsupported platform ${key}. Supported: ${Object.keys(PLATFORMS).join(", ")}`);
  process.exit(1);
}

function getCacheDir(version) {
  return path.join(os.homedir(), ".cache", "quickcrawl-mcp", version);
}

function getBinaryPath(version) {
  return path.join(getCacheDir(version), BINARY_NAME + (process.platform === "win32" ? ".exe" : ""));
}

function compareVersions(v1, v2) {
  const parts1 = v1.split(".").map(Number);
  const parts2 = v2.split(".").map(Number);
  for (let i = 0; i < Math.max(parts1.length, parts2.length); i++) {
    const p1 = parts1[i] || 0;
    const p2 = parts2[i] || 0;
    if (p1 > p2) return 1;
    if (p1 < p2) return -1;
  }
  return 0;
}

async function getLatestVersion() {
  return new Promise((resolve) => {
    const url = `https://api.github.com/repos/${REPO}/releases/latest`;
    https.get(url, { headers: { "User-Agent": "quickcrawl-mcp" } }, (res) => {
      let data = "";
      res.on("data", chunk => data += chunk);
      res.on("end", () => {
        try {
          const json = JSON.parse(data);
          resolve(json.tag_name?.replace(/^v/, "") || packageVersion);
        } catch {
          resolve(packageVersion);
        }
      });
    }).on("error", () => resolve(packageVersion));
  });
}

function download(url, dest) {
  return new Promise((resolve, reject) => {
    const file = fs.createWriteStream(dest);
    const protocol = url.startsWith("https") ? https : require("http");

    protocol.get(url, { headers: { "User-Agent": "quickcrawl-mcp" } }, (response) => {
      if ([301, 302, 307, 308].includes(response.statusCode)) {
        if (!response.headers.location) {
          file.close();
          fs.unlinkSync(dest);
          reject(new Error(`redirect missing location for ${url}`));
          return;
        }
        file.close();
        fs.unlinkSync(dest);
        download(response.headers.location, dest).then(resolve).catch(reject);
        return;
      }
      if (!response.statusCode || response.statusCode < 200 || response.statusCode >= 300) {
        file.close();
        fs.unlinkSync(dest);
        reject(new Error(`download failed with status ${response.statusCode}`));
        return;
      }
      response.pipe(file);
      file.on("finish", () => { file.close(); resolve(); });
    }).on("error", (err) => {
      try { fs.unlinkSync(dest); } catch {}
      reject(err);
    });
  });
}

async function extractTarGz(tarPath, destDir) {
  return new Promise((resolve, reject) => {
    const result = spawnSync("tar", ["-xzf", tarPath, "-C", destDir], { stdio: "pipe" });
    if (result.status === 0) resolve();
    else reject(new Error("tar extraction failed"));
  });
}

async function extractZip(zipPath, destDir) {
  return new Promise((resolve, reject) => {
    const result = process.platform === "win32"
      ? spawnSync("powershell.exe", [
          "-NoProfile",
          "-NonInteractive",
          "-Command",
          `Expand-Archive -LiteralPath '${zipPath.replace(/'/g, "''")}' -DestinationPath '${destDir.replace(/'/g, "''")}' -Force`,
        ], { stdio: "pipe" })
      : spawnSync("unzip", ["-o", zipPath, "-d", destDir], { stdio: "pipe" });
    if (result.status === 0) resolve();
    else reject(new Error("unzip failed"));
  });
}

async function ensureBinary(version, forceUpdate) {
  const binaryPath = getBinaryPath(version);
  const cacheDir = getCacheDir(version);

  if (fs.existsSync(binaryPath)) {
    if (forceUpdate) {
      console.error(`Cached v${version} found but newer version available. Re-downloading...`);
      fs.rmSync(binaryPath, { force: true });
    } else {
      fs.chmodSync(binaryPath, 0o755);
      return binaryPath;
    }
  }

  console.error(`Downloading quickcrawl-mcp v${version} for ${key}...`);

  const url = `https://github.com/${REPO}/releases/download/v${version}/${getArchiveName(version)}`;

  if (!fs.existsSync(cacheDir)) {
    fs.mkdirSync(cacheDir, { recursive: true });
  }

  const archivePath = path.join(cacheDir, getArchiveName(version));

  try {
    await download(url, archivePath);

    if (platform.ext === ".zip") {
      await extractZip(archivePath, cacheDir);
    } else {
      await extractTarGz(archivePath, cacheDir);
    }

    fs.unlinkSync(archivePath);

    const finalPath = getBinaryPath(version);
    const extractedBinary = path.join(cacheDir, platform.binary);
    if (fs.existsSync(extractedBinary)) {
      fs.chmodSync(finalPath, 0o755);
      return finalPath;
    }

    throw new Error("Binary not found after extraction");
  } catch (err) {
    console.error(`Failed to download: ${err.message}`);
    console.error(`\nTo install manually:`);
    console.error(`  go install github.com/${REPO}/cmd/mcp@latest`);
    process.exit(1);
  }
}

async function main() {
  const latestVersion = await getLatestVersion();
  const cachedVersion = packageVersion;

  const forceUpdate = compareVersions(latestVersion, cachedVersion) > 0;

  if (forceUpdate) {
    console.error(`New version available: v${latestVersion} (current: v${cachedVersion})`);
  }

  const binaryPath = await ensureBinary(latestVersion, forceUpdate);
  const args = process.argv.slice(2);
  const result = spawnSync(binaryPath, args, { stdio: "inherit" });
  process.exit(result.status ?? 0);
}

main();
