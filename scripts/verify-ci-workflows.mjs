#!/usr/bin/env node
import fs from "node:fs";
import { execFileSync } from "node:child_process";

const workflowPath = ".github/workflows/build.yml";
const text = fs.readFileSync(workflowPath, "utf8");

const required = [
  "name: Build and Release",
  "push:",
  "branches:",
  "- master",
  "tags:",
  "- 'v*'",
  "pull_request:",
  "workflow_dispatch:",
  "node-version: '24'",
  "platform: linux",
  "platform: darwin",
  "platform: windows",
  "arch: amd64",
  "arch: arm64",
  "wails build -platform ${{ matrix.platform }}/${{ matrix.arch }}",
  "CodeAgentLens.exe",
  "CodeAgentLens-${{ steps.get_version.outputs.version }}-${{ matrix.platform }}-${{ matrix.arch }}.zip",
  "if: startsWith(github.ref, 'refs/tags/')",
];

const failures = [];
for (const literal of required) {
  if (!text.includes(literal)) {
    failures.push(`${workflowPath}: missing literal ${literal}`);
  }
}

const pushBlock = text.match(/on:\s*\n([\s\S]*?)\npermissions:/)?.[1] ?? "";
const pushSection = pushBlock.match(/  push:\s*\n([\s\S]*?)(?=\n  [a-z_]+:|\n\S|$)/)?.[1] ?? "";
if (!pushSection.includes("branches:") || !pushSection.includes("- master")) {
  failures.push(`${workflowPath}: push trigger must include branch master`);
}

const trackedFiles = new Set(
  execFileSync("git", ["ls-files"], { encoding: "utf8" })
    .split(/\r?\n/)
    .filter(Boolean),
);
for (const file of [
  "cmd/desktop/build/appicon.png",
  "cmd/desktop/build/appicon.svg",
  "cmd/desktop/build/darwin/Info.plist",
  "cmd/desktop/build/windows/icon.ico",
  "cmd/desktop/build/windows/wails.exe.manifest",
]) {
  if (!trackedFiles.has(file)) {
    failures.push(`${file}: must be tracked for Wails CI builds`);
  }
}

if (failures.length > 0) {
  console.error(failures.join("\n"));
  process.exit(1);
}

console.log("ci workflow checks passed");
