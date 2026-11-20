import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { dirname, join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";
import { bodyViolations, commitMessageViolations, footerMarker } from "./check-pr-body.mjs";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");

function message(sha) {
  return execFileSync("git", ["log", "-1", "--format=%B", sha], { cwd: root, encoding: "utf8" });
}

test("a pull request body with the agent footer comment fails", () => {
  const violations = bodyViolations(`Summary\n${footerMarker}_BEGIN -->\n`);
  assert.equal(violations.length, 1);
  assert.match(violations[0], /agent footer comment/);
  assert.match(violations[0], /Remove the footer/);
});

test("an ordinary pull request body passes", () => {
  assert.deepEqual(bodyViolations("Explain the change and the check you ran."), []);
});

test("prose that names the footer marker passes", () => {
  assert.deepEqual(bodyViolations("The check rejects a CURSOR_AGENT_PR_BODY comment."), []);
});

test("27ea773 Cursor Agent trailer fails", () => {
  const sha = "27ea77367e171cf131380c56fd5bbd48276fc79f";
  const violations = commitMessageViolations(message(sha), sha);
  assert.equal(violations.length, 1);
  assert.match(
    violations[0],
    /27ea77367e171cf131380c56fd5bbd48276fc79f contains "Co-authored-by: Cursor Agent <cursoragent@cursor\.com>"/,
  );
  assert.match(violations[0], /Commit as Andrejs Strazds and remove the Cursor agent trailer/);
});

test("6add023 Cursor Agent trailer fails", () => {
  const sha = "6add0230ef82e6f02d0521dcb99eab4a51719d78";
  const violations = commitMessageViolations(message(sha), sha);
  assert.equal(violations.length, 1);
  assert.match(violations[0], /Commit as Andrejs Strazds and remove the Cursor agent trailer/);
});

test("a Cursor trailer without the Agent display name fails", () => {
  const violations = commitMessageViolations(
    "Co-authored-by: Cursor <cursoragent@cursor.com>\n",
    "short-name",
  );
  assert.equal(violations.length, 1);
  assert.match(violations[0], /short-name contains "Co-authored-by: Cursor <cursoragent@cursor\.com>"/);
});

test("80e629b has no Cursor agent trailer", () => {
  const sha = "80e629b93f635b0fab4c648e7a0e0254cf901522";
  assert.deepEqual(commitMessageViolations(message(sha), sha), []);
});
