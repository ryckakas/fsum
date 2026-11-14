// Replays testdata/golden.txt through the engine's own Math.sumPrecise and fails on any disagreement.
// Node 26 ships it behind a flag: node --js-sum-precise scripts/crosscheck.mjs [golden.txt]
import { readFileSync } from "node:fs";

if (typeof Math.sumPrecise !== "function") {
  console.error("Math.sumPrecise is unavailable: run with `node --js-sum-precise`.");
  process.exit(2);
}

const view = new DataView(new ArrayBuffer(8));

function fromHex(hex) {
  view.setBigUint64(0, BigInt(`0x${hex}`));
  return view.getFloat64(0);
}

function toHex(x) {
  view.setFloat64(0, x);
  return view.getBigUint64(0).toString(16).padStart(16, "0");
}

function same(a, b) {
  return Number.isNaN(a) ? Number.isNaN(b) : Object.is(a, b);
}

const path = process.argv[2] ?? "testdata/golden.txt";
let checked = 0;
let failed = 0;

for (const line of readFileSync(path, "utf8").split("\n")) {
  if (line === "" || line.startsWith("#")) continue;
  const [name, want, inputs] = line.split("\t");
  const xs = inputs === "" ? [] : inputs.split(" ").map(fromHex);
  const got = Math.sumPrecise(xs);
  checked++;
  if (!same(fromHex(want), got)) {
    failed++;
    console.error(`${name}: corpus ${want}, Math.sumPrecise ${toHex(got)}`);
  }
}

console.log(`${checked} cases checked against Math.sumPrecise (${process.version}), ${failed} disagreements`);
process.exit(failed === 0 ? 0 : 1);
