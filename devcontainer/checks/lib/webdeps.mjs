// webdeps.mjs is the dependency policy checker of web-deps.sh (v0-T258). Run it in the leaf's top folder:
//
//   node webdeps.mjs check <policy.json>   checks package.json and package-lock.json against the policy
//   node webdeps.mjs audit <policy.json>   prints the audit tools the policy names, one per line
//
// It reads files and runs nothing of the leaf, and uses node's own modules only. The policy is baked into the
// image and the script passes its path; the checker never looks for a policy in the leaf.
import { readFileSync } from "node:fs";

const fail = (message) => {
  console.log(`deps policy: ${message}`);
  process.exit(1);
};

const readJSON = (path) => {
  try {
    return JSON.parse(readFileSync(path, "utf8"));
  } catch (e) {
    return fail(`${path}: ${e.code === "ENOENT" ? `no ${path}` : e.message}`);
  }
};

const plain = /^(\d+)\.(\d+)\.(\d+)$/;
const pinned = /^~?(\d+)\.(\d+)\.(\d+)$/;

function load(path) {
  const p = readJSON(path);
  const known = ["schema", "pin", "minimum", "audit"];
  const extra = Object.keys(p).filter((k) => !known.includes(k));
  if (extra.length > 0) fail(`${path} has the unknown field ${extra[0]}`);
  if (p.schema !== "ygg.deps/v1") fail(`${path} has schema ${JSON.stringify(p.schema)}, want ygg.deps/v1`);
  if (p.pin !== "minor") fail(`${path} has pin ${JSON.stringify(p.pin)}, want minor`);
  for (const [name, v] of Object.entries(p.minimum ?? {})) {
    if (!plain.test(v)) fail(`${path}: the minimum of ${name} is ${JSON.stringify(v)}, want x.y.z`);
  }
  for (const a of p.audit ?? []) {
    if (a !== "npm-audit" && a !== "osv") fail(`${path} names the audit tool ${JSON.stringify(a)}; the web ladder knows npm-audit and osv`);
  }
  return p;
}

// below says whether version is lower than minimum (x.y.z). A pre-release of the minimum is lower than it.
function below(version, minimum) {
  const m = /^(\d+)\.(\d+)\.(\d+)(-.*)?/.exec(version);
  if (!m) return true;
  const have = m.slice(1, 4).map(Number);
  const want = minimum.split(".").map(Number);
  for (let i = 0; i < 3; i++) if (have[i] !== want[i]) return have[i] < want[i];
  return m[4] !== undefined;
}

function check(p) {
  const pkg = readJSON("package.json");
  const problems = [];
  for (const section of ["dependencies", "devDependencies", "optionalDependencies"]) {
    for (const [name, spec] of Object.entries(pkg[section] ?? {})) {
      if (!pinned.test(spec)) problems.push(`package.json: ${name} ${spec} is not pinned to a minor line (x.y.z or ~x.y.z)`);
    }
  }
  let lock;
  try {
    lock = JSON.parse(readFileSync("package-lock.json", "utf8"));
  } catch {
    problems.push("package-lock.json: no package-lock.json; the packages must be locked");
  }
  const marker = "node_modules/";
  for (const [path, entry] of Object.entries(lock?.packages ?? {})) {
    const at = path.lastIndexOf(marker);
    if (at < 0) continue;
    const name = path.slice(at + marker.length);
    const min = p.minimum?.[name];
    if (min !== undefined && below(String(entry.version), min)) {
      const label = path.indexOf(marker) === at ? name : path;
      problems.push(`package-lock.json: ${label} ${entry.version} is below the minimum ${min}`);
    }
  }
  problems.sort();
  for (const s of problems) console.log(s);
  if (problems.length > 0) fail(`${problems.length} problem(s)`);
  console.log("deps policy: ok");
}

const [command, policyPath, ...rest] = process.argv.slice(2);
if (!policyPath || rest.length > 0) fail("usage: webdeps.mjs check|audit <policy.json>");
const policy = load(policyPath);
if (command === "check") check(policy);
else if (command === "audit") for (const a of policy.audit ?? []) console.log(a);
else fail(`unknown command ${JSON.stringify(command)}`);
