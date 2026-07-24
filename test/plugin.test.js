import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";
import { access, cp, mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import test from "node:test";

const repoRoot = dirname(dirname(fileURLToPath(import.meta.url)));
const codexPath = join(repoRoot, "node_modules", ".bin", "codex");

function runCodex(configRoot, args) {
  return spawnSync(codexPath, args, {
    cwd: repoRoot,
    encoding: "utf8",
    env: { ...process.env, CODEX_HOME: configRoot },
  });
}

test("Codex installs the distributable plugin and discovers its skill", async () => {
  const fixtureRoot = await mkdtemp(join(tmpdir(), "fjgo-plugin-"));
  const marketplaceRoot = join(fixtureRoot, "marketplace");
  const pluginRoot = join(marketplaceRoot, "plugins", "fjgo");
  const configRoot = join(fixtureRoot, "codex");
  try {
    for (const path of [".codex-plugin", "assets", "skills"]) {
      await cp(join(repoRoot, path), join(pluginRoot, path), { recursive: true });
    }
    await mkdir(join(marketplaceRoot, ".agents", "plugins"), { recursive: true });
    await mkdir(configRoot, { recursive: true });
    await writeFile(
      join(marketplaceRoot, ".agents", "plugins", "marketplace.json"),
      JSON.stringify({
        name: "fjgo-test",
        interface: { displayName: "fjgo Test" },
        plugins: [{
          name: "fjgo",
          source: { source: "local", path: "./plugins/fjgo" },
          policy: { installation: "AVAILABLE", authentication: "ON_USE" },
          category: "Developer Tools",
        }],
      }),
    );

    const manifest = JSON.parse(
      await readFile(join(repoRoot, ".codex-plugin", "plugin.json"), "utf8"),
    );
    const packageJSON = JSON.parse(await readFile(join(repoRoot, "package.json"), "utf8"));
    assert.equal(manifest.version, packageJSON.version);
    assert.equal(manifest.apps, undefined);
    assert.equal(manifest.mcpServers, undefined);
    assert.equal(manifest.hooks, undefined);
    for (const path of [".app.json", ".mcp.json", "hooks"]) {
      await assert.rejects(access(join(repoRoot, path)), { code: "ENOENT" });
    }

    const added = runCodex(configRoot, [
      "plugin", "marketplace", "add", marketplaceRoot, "--json",
    ]);
    assert.equal(
      added.status,
      0,
      `marketplace setup failed:\n${added.stdout}${added.stderr}`,
    );

    const installed = runCodex(configRoot, [
      "plugin", "add", "fjgo@fjgo-test", "--json",
    ]);
    assert.equal(
      installed.status,
      0,
      `plugin installation failed:\n${installed.stdout}${installed.stderr}`,
    );
    const installation = JSON.parse(installed.stdout);
    assert.deepEqual(
      { name: installation.name, version: installation.version },
      { name: "fjgo", version: packageJSON.version },
    );

    const prompt = runCodex(configRoot, [
      "-C", repoRoot, "debug", "prompt-input", "Inspect this Forgejo repository.",
    ]);
    assert.equal(
      prompt.status,
      0,
      `Codex prompt discovery failed:\n${prompt.stdout}${prompt.stderr}`,
    );
    assert.match(prompt.stdout, /Forgejo repository operations through the fjgo CLI/);
  } finally {
    await rm(fixtureRoot, { recursive: true, force: true });
  }
});
