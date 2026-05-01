/**
 * bc-cli postinstall script
 *
 * Downloads the correct bc binary for the current platform from GitHub Releases.
 * Uses only Node.js built-ins — no external dependencies.
 */

import { execFileSync } from "node:child_process";
import { mkdirSync, chmodSync, writeFileSync } from "node:fs";
import { createGunzip } from "node:zlib";
import https from "node:https";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const BIN_DIR = join(__dirname, "bin");
const BIN_PATH = join(BIN_DIR, "bc");
const REPO = "rpuneet/mycel";

function getPlatform() {
  const platform = process.platform;
  const arch = process.arch;

  const osMap = {
    darwin: "darwin",
    linux: "linux",
  };

  const archMap = {
    x64: "amd64",
    arm64: "arm64",
  };

  const os = osMap[platform];
  const cpu = archMap[arch];

  if (!os || !cpu) {
    throw new Error(
      `Unsupported platform: ${platform}/${arch}. ` +
        `bc supports: macOS (amd64, arm64), Linux (amd64).`
    );
  }

  // Linux arm64 not currently built by goreleaser
  if (os === "linux" && cpu === "arm64") {
    throw new Error(
      `Unsupported platform: linux/arm64. ` +
        `bc currently supports: macOS (amd64, arm64), Linux (amd64).`
    );
  }

  return { os, arch: cpu };
}

function httpsGet(url) {
  return new Promise((resolve, reject) => {
    https
      .get(url, { headers: { "User-Agent": "bc-cli-installer" } }, (res) => {
        if (
          res.statusCode >= 300 &&
          res.statusCode < 400 &&
          res.headers.location
        ) {
          httpsGet(res.headers.location).then(resolve, reject);
          return;
        }
        if (res.statusCode !== 200) {
          res.resume();
          reject(new Error(`HTTP ${res.statusCode} fetching ${url}`));
          return;
        }
        resolve(res);
      })
      .on("error", reject);
  });
}

function httpsGetJSON(url) {
  return new Promise((resolve, reject) => {
    httpsGet(url)
      .then((res) => {
        let data = "";
        res.on("data", (chunk) => (data += chunk));
        res.on("end", () => {
          try {
            resolve(JSON.parse(data));
          } catch (e) {
            reject(new Error(`Invalid JSON from ${url}: ${e.message}`));
          }
        });
        res.on("error", reject);
      })
      .catch(reject);
  });
}

async function getVersion() {
  // Try to get latest release tag from GitHub
  try {
    const release = await httpsGetJSON(
      `https://api.github.com/repos/${REPO}/releases/latest`
    );
    if (release.tag_name) {
      return release.tag_name.replace(/^v/, "");
    }
  } catch {
    // Fall through to package version
  }

  // Fall back to package.json version
  const { readFileSync } = await import("node:fs");
  const pkg = JSON.parse(readFileSync(join(__dirname, "package.json"), "utf8"));
  return pkg.version;
}

/**
 * Extract a .tar.gz stream. Minimal tar parser — we only need to extract
 * one file (the bc binary) and want zero dependencies.
 */
async function extractTarGz(stream, destPath) {
  const gunzip = createGunzip();
  const chunks = [];

  await new Promise((resolve, reject) => {
    stream.pipe(gunzip);
    gunzip.on("data", (chunk) => chunks.push(chunk));
    gunzip.on("end", resolve);
    gunzip.on("error", reject);
    stream.on("error", reject);
  });

  const tar = Buffer.concat(chunks);
  let offset = 0;

  while (offset < tar.length) {
    // tar header is 512 bytes
    if (offset + 512 > tar.length) break;

    const header = tar.subarray(offset, offset + 512);

    // Check for end-of-archive (two 512-byte blocks of zeros)
    if (header.every((b) => b === 0)) break;

    // Extract filename (bytes 0-99, null-terminated)
    const nameEnd = header.indexOf(0, 0);
    const name = header
      .subarray(0, Math.min(nameEnd, 100))
      .toString("utf8");

    // Extract file size (bytes 124-135, octal, null/space terminated)
    const sizeStr = header.subarray(124, 136).toString("utf8").trim();
    const size = parseInt(sizeStr, 8) || 0;

    // Extract type flag (byte 156)
    const typeFlag = header[156];

    offset += 512; // Move past header

    // Type '0' or '\0' = regular file
    if (
      (typeFlag === 48 || typeFlag === 0) &&
      (name === "bc" || name.endsWith("/bc"))
    ) {
      const fileData = tar.subarray(offset, offset + size);
      mkdirSync(dirname(destPath), { recursive: true });
      writeFileSync(destPath, fileData);
      chmodSync(destPath, 0o755);
      return true;
    }

    // Advance past file data (padded to 512-byte boundary)
    offset += Math.ceil(size / 512) * 512;
  }

  return false;
}

async function install() {
  const { os, arch } = getPlatform();
  const version = await getVersion();

  const filename = `bc_${version}_${os}_${arch}.tar.gz`;
  const url = `https://github.com/${REPO}/releases/download/v${version}/${filename}`;

  console.log(`bc-cli: downloading bc v${version} for ${os}/${arch}...`);
  console.log(`bc-cli: ${url}`);

  const res = await httpsGet(url);
  const found = await extractTarGz(res, BIN_PATH);

  if (!found) {
    throw new Error("Could not find 'bc' binary in the downloaded archive.");
  }

  // Verify the binary works (using execFileSync — no shell, no injection risk)
  try {
    const output = execFileSync(BIN_PATH, ["version"], {
      encoding: "utf8",
      timeout: 10000,
    }).trim();
    console.log(`bc-cli: installed successfully — ${output}`);
  } catch {
    console.log(
      "bc-cli: binary installed but 'bc version' check failed (this may be fine on CI)."
    );
  }
}

install().catch((err) => {
  console.error(`\nbc-cli: installation failed: ${err.message}\n`);
  console.error("You can install bc manually:");
  console.error("");
  console.error("  # macOS (Homebrew)");
  console.error("  brew install rpuneet/mycel/bc-infra");
  console.error("");
  console.error("  # From source");
  console.error(
    "  git clone https://github.com/rpuneet/mycel && cd bc && make install-local-bc"
  );
  console.error("");
  console.error("  # Direct download");
  console.error("  https://github.com/rpuneet/mycel/releases/latest");
  console.error("");

  // Exit 0 so npm install doesn't fail — the placeholder script will tell
  // the user what happened if they try to run bc.
  process.exit(0);
});
