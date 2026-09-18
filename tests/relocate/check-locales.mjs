// Throwaway validator: checks the relocate i18n keys across all locales.
// Uses Node's JSON parser, i.e. the same one the frontend build relies on.
import { readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";

const base = "D:/Desktop/cloudreve/assets/public/locales";
const required = [
  "relocation",
  "relocateSelectTarget",
  "relocateSummary",
  "relocateHint",
  "relocateNoTarget",
];

let bad = 0;
for (const lang of readdirSync(base)) {
  const file = join(base, lang, "application.json");
  let json;
  try {
    json = JSON.parse(readFileSync(file, "utf8"));
  } catch (e) {
    console.log(`${lang.padEnd(6)} JSON ERROR: ${e.message}`);
    bad++;
    continue;
  }

  const fm = json.fileManager ?? {};
  const missing = required.filter((k) => !fm[k]);
  const hasStray = Object.prototype.hasOwnProperty.call(fm, "relocate");

  console.log(
    `${lang.padEnd(6)} missing=[${missing.join(",")}] stray_relocate_key=${hasStray} relocation="${fm.relocation ?? ""}"`,
  );
  if (missing.length > 0 || hasStray) bad++;
}
console.log(`locales with problems: ${bad}`);
