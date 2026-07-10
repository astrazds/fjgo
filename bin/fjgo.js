#!/usr/bin/env node

import { run } from "../lib/launcher.js";

run(process.argv.slice(2)).catch((error) => {
  process.stdout.write(`error: unable to run fjgo: ${error.message}\n`);
  process.stdout.write("help: check network access or install a native fjgo release\n");
  process.exitCode = 1;
});
