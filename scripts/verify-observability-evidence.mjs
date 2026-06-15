#!/usr/bin/env node
import fs from "node:fs";

const evidencePath = process.argv[2];
if (!evidencePath) {
  console.error("usage: node scripts/verify-observability-evidence.mjs <evidence.md>");
  process.exit(2);
}
if (!fs.existsSync(evidencePath)) {
  console.error(`evidence file missing: ${evidencePath}`);
  process.exit(1);
}

const text = fs.readFileSync(evidencePath, "utf8");
const requiredSections = [
  "sourcePlanHash",
  "contractHash",
  "commands",
  "manualVerification",
  "notDone",
  "residualRisks",
];
const failures = [];
for (const section of requiredSections) {
  if (!text.includes(section)) {
    failures.push(`missing section or field: ${section}`);
  }
}

for (let i = 1; i <= 12; i++) {
  const id = `G${String(i).padStart(3, "0")}`;
  if (!text.includes(id)) failures.push(`missing task evidence: ${id}`);
}
for (let i = 1; i <= 20; i++) {
  const id = `A${String(i).padStart(3, "0")}`;
  if (!text.includes(id)) failures.push(`missing acceptance evidence: ${id}`);
}
for (let i = 1; i <= 13; i++) {
  const id = `C${String(i).padStart(3, "0")}`;
  if (!text.includes(id)) failures.push(`missing command evidence: ${id}`);
}
for (let i = 1; i <= 5; i++) {
  const id = `M${String(i).padStart(3, "0")}`;
  if (!text.includes(id)) failures.push(`missing manual verification evidence: ${id}`);
}
for (let i = 1; i <= 5; i++) {
  const id = `ND${String(i).padStart(3, "0")}`;
  if (!text.includes(id)) failures.push(`missing not-done evidence: ${id}`);
}
if (/completion_blocked:\s*true/i.test(text)) {
  failures.push("residualRisks contains completion_blocked=true");
}

if (failures.length > 0) {
  console.error(failures.join("\n"));
  process.exit(1);
}

console.log(`observability evidence checks passed file=${evidencePath}`);
