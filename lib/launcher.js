import { createHash } from "node:crypto";
import { spawn } from "node:child_process";
import { chmod, mkdir, readFile, rename, rm, writeFile } from "node:fs/promises";
import { homedir } from "node:os";
import { basename, dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { gunzipSync } from "node:zlib";

const releaseBase = "https://github.com/astrazds/fjgo/releases/download";

export function releaseTarget(platform = process.platform, arch = process.arch) {
  const platforms = { darwin: "darwin", linux: "linux" };
  const architectures = { arm64: "arm64", x64: "amd64" };
  if (!platforms[platform] || !architectures[arch]) {
    throw new Error(`unsupported platform ${platform}/${arch}; supported platforms are linux and macOS on x64 or arm64`);
  }
  return { os: platforms[platform], arch: architectures[arch] };
}

export function checksumFor(checksums, archiveName) {
  for (const line of checksums.split(/\r?\n/)) {
    const match = line.trim().match(/^([a-fA-F0-9]{64})\s+\*?(.+)$/);
    if (match && basename(match[2]) === archiveName) {
      return match[1].toLowerCase();
    }
  }
  throw new Error(`release checksums do not contain ${archiveName}`);
}

function tarString(block, start, length) {
  return block.subarray(start, start + length).toString("utf8").replace(/\0.*$/, "");
}

export function extractBinary(archive, expectedPath) {
  const tar = gunzipSync(archive);
  for (let offset = 0; offset + 512 <= tar.length; ) {
    const header = tar.subarray(offset, offset + 512);
    if (header.every((byte) => byte === 0)) break;
    const name = tarString(header, 0, 100);
    const prefix = tarString(header, 345, 155);
    const path = prefix ? `${prefix}/${name}` : name;
    const sizeText = tarString(header, 124, 12).trim();
    const size = Number.parseInt(sizeText || "0", 8);
    if (!Number.isSafeInteger(size) || size < 0) throw new Error("release archive has an invalid entry size");
    const dataStart = offset + 512;
    const dataEnd = dataStart + size;
    if (dataEnd > tar.length) throw new Error("release archive is truncated");
    if (path === expectedPath) return Buffer.from(tar.subarray(dataStart, dataEnd));
    offset = dataStart + Math.ceil(size / 512) * 512;
  }
  throw new Error(`release archive does not contain ${expectedPath}`);
}

async function download(url, fetchImpl) {
  const response = await fetchImpl(url, { redirect: "follow", signal: AbortSignal.timeout(30_000) });
  if (!response.ok) throw new Error(`download failed with HTTP ${response.status} for ${url}`);
  return Buffer.from(await response.arrayBuffer());
}

export async function ensureBinary(options = {}) {
  const packagePath = fileURLToPath(new URL("../package.json", import.meta.url));
  const packageJSON = JSON.parse(await readFile(packagePath, "utf8"));
  const version = options.version ?? packageJSON.version;
  const tag = version.startsWith("v") ? version : `v${version}`;
  const target = releaseTarget(options.platform, options.arch);
  const archiveName = `fjgo_${tag}_${target.os}_${target.arch}.tar.gz`;
  const entryName = `fjgo_${tag}_${target.os}_${target.arch}/fjgo`;
  const cacheRoot = options.cacheDir ?? process.env.FJGO_NPX_CACHE_DIR ?? join(process.env.XDG_CACHE_HOME || join(homedir(), ".cache"), "fjgo");
  const binary = join(cacheRoot, tag, `${target.os}-${target.arch}`, "fjgo");

  try {
    await chmod(binary, 0o755);
    return binary;
  } catch (error) {
    if (error.code !== "ENOENT") throw error;
  }

  const base = (options.baseUrl ?? process.env.FJGO_NPX_RELEASE_BASE ?? releaseBase).replace(/\/$/, "");
  const fetchImpl = options.fetchImpl ?? fetch;
  const [archive, checksumsBuffer] = await Promise.all([
    download(`${base}/${tag}/${archiveName}`, fetchImpl),
    download(`${base}/${tag}/checksums.txt`, fetchImpl),
  ]);
  const expected = checksumFor(checksumsBuffer.toString("utf8"), archiveName);
  const actual = createHash("sha256").update(archive).digest("hex");
  if (actual !== expected) throw new Error(`checksum mismatch for ${archiveName}`);

  const contents = extractBinary(archive, entryName);
  await mkdir(dirname(binary), { recursive: true });
  const temporary = `${binary}.${process.pid}.tmp`;
  try {
    await writeFile(temporary, contents, { mode: 0o755 });
    await rename(temporary, binary);
  } finally {
    await rm(temporary, { force: true });
  }
  return binary;
}

export async function run(args) {
  const binary = await ensureBinary();
  const child = spawn(binary, args, { stdio: "inherit", env: process.env });
  await new Promise((resolve, reject) => {
    child.once("error", reject);
    child.once("exit", (code, signal) => {
      if (signal) process.kill(process.pid, signal);
      else process.exitCode = code ?? 1;
      resolve();
    });
  });
}
