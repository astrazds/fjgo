import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { mkdtemp, readFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { gzipSync } from "node:zlib";

import { checksumFor, ensureBinary, extractBinary, releaseTarget } from "../lib/launcher.js";

function tarArchive(path, contents) {
  const data = Buffer.from(contents);
  const header = Buffer.alloc(512);
  header.write(path, 0, 100, "utf8");
  header.write("0000755\0", 100, 8, "ascii");
  header.write("0000000\0", 108, 8, "ascii");
  header.write("0000000\0", 116, 8, "ascii");
  header.write(`${data.length.toString(8).padStart(11, "0")}\0`, 124, 12, "ascii");
  header.write("00000000000\0", 136, 12, "ascii");
  header.fill(0x20, 148, 156);
  header[156] = "0".charCodeAt(0);
  header.write("ustar\0", 257, 6, "ascii");
  const sum = header.reduce((total, byte) => total + byte, 0);
  header.write(`${sum.toString(8).padStart(6, "0")}\0 `, 148, 8, "ascii");
  const padding = Buffer.alloc(Math.ceil(data.length / 512) * 512 - data.length);
  return gzipSync(Buffer.concat([header, data, padding, Buffer.alloc(1024)]));
}

test("maps supported release targets", () => {
  assert.deepEqual(releaseTarget("linux", "x64"), { os: "linux", arch: "amd64" });
  assert.deepEqual(releaseTarget("darwin", "arm64"), { os: "darwin", arch: "arm64" });
  assert.throws(() => releaseTarget("win32", "x64"), /unsupported platform/);
});

test("finds checksums by archive basename", () => {
  const name = "fjgo_v1.3.0_linux_amd64.tar.gz";
  assert.equal(checksumFor(`${"a".repeat(64)}  ./${name}\n`, name), "a".repeat(64));
  assert.throws(() => checksumFor("", name), /do not contain/);
});

test("extracts and caches a verified release binary", async () => {
  const version = "v9.8.7";
  const name = `fjgo_${version}_linux_amd64.tar.gz`;
  const entry = `fjgo_${version}_linux_amd64/fjgo`;
  const archive = tarArchive(entry, "test binary");
  assert.equal(extractBinary(archive, entry).toString(), "test binary");
  const checksum = createHash("sha256").update(archive).digest("hex");
  const cacheDir = await mkdtemp(join(tmpdir(), "fjgo-npx-"));
  let requests = 0;
  const fetchImpl = async (url) => {
    requests += 1;
    const body = url.endsWith("checksums.txt") ? `${checksum}  ./${name}\n` : archive;
    return new Response(body, { status: 200 });
  };

  try {
    const binary = await ensureBinary({ version, platform: "linux", arch: "x64", cacheDir, baseUrl: "https://example.invalid", fetchImpl });
    assert.equal((await readFile(binary)).toString(), "test binary");
    assert.equal(await ensureBinary({ version, platform: "linux", arch: "x64", cacheDir, fetchImpl }), binary);
    assert.equal(requests, 2);
  } finally {
    await rm(cacheDir, { recursive: true, force: true });
  }
});
