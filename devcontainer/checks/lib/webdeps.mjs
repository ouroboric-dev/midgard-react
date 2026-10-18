// webdeps.mjs is the dependency policy checker of web-deps.sh (v0-T258). Run it in the leaf's top folder:
//
//   node webdeps.mjs check <policy.json>   checks package.json and package-lock.json against the policy
//   node webdeps.mjs installed <policy.json>  after npm ci: checks the minimums against node_modules
//   node webdeps.mjs audit <policy.json>   prints the audit tools the policy names, one per line
//
// It reads files and runs nothing of the leaf, and uses node's own modules only. The policy is baked into the
// image and the script passes its path; the checker never looks for a policy in the leaf.
import { existsSync, readFileSync, readdirSync } from "node:fs";

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
  // npm ci prefers npm-shrinkwrap.json to package-lock.json, and the rungs protect only the latter's reader here.
  if (existsSync("npm-shrinkwrap.json")) problems.push("npm-shrinkwrap.json: not allowed; npm ci would read it in place of package-lock.json");
  let lock;
  try {
    lock = JSON.parse(readFileSync("package-lock.json", "utf8"));
  } catch (e) {
    problems.push(e.code === "ENOENT" ? "package-lock.json: no package-lock.json; the packages must be locked" : `package-lock.json: not JSON (${e.message})`);
  }
  if (lock !== undefined) {
    if (lock === null || typeof lock !== "object" || Array.isArray(lock)) {
      problems.push("package-lock.json: not a JSON object");
      lock = undefined;
    } else if (!(lock.lockfileVersion >= 2) || lock.packages === null || typeof lock.packages !== "object") {
      problems.push("package-lock.json: needs lockfileVersion 2 or 3 with a packages map, or the minimums cannot be read");
      lock = undefined;
    }
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

// walk lists the packages in a node_modules folder, nested ones too: [path, directory].
function* walk(dir, path) {
  let entries;
  try {
    entries = readdirSync(dir, { withFileTypes: true });
  } catch {
    return;
  }
  for (const e of entries) {
    if (!e.isDirectory() || e.name === ".bin") continue;
    if (e.name.startsWith("@")) {
      for (const sub of readdirSync(`${dir}/${e.name}`, { withFileTypes: true })) {
        if (sub.isDirectory()) yield* pkg(`${dir}/${e.name}/${sub.name}`, `${path}${e.name}/${sub.name}`);
      }
    } else {
      yield* pkg(`${dir}/${e.name}`, `${path}${e.name}`);
    }
  }
}
function* pkg(dir, path) {
  yield [path, dir];
  yield* walk(`${dir}/node_modules`, `${path}/node_modules/`);
}

// installed checks the minimums against what npm ci put in node_modules: the version field of each package's own
// package.json, which comes from the integrity-checked tarball. The lock file's version field is the agent's text.
function installed(p) {
  const problems = [];
  for (const [path, dir] of walk("node_modules", "node_modules/")) {
    const name = path.slice(path.lastIndexOf("node_modules/") + "node_modules/".length);
    const min = p.minimum?.[name];
    if (min === undefined) continue;
    let version;
    try {
      version = JSON.parse(readFileSync(`${dir}/package.json`, "utf8")).version;
    } catch {
      continue;
    }
    if (below(String(version), min)) problems.push(`${path} ${version} is installed, below the minimum ${min}`);
  }
  problems.sort();
  for (const s of problems) console.log(s);
  if (problems.length > 0) fail(`${problems.length} problem(s) in node_modules`);
  console.log("deps policy: installed packages ok");
}

const [command, policyPath, ...rest] = process.argv.slice(2);
if (!policyPath || rest.length > 0) fail("usage: webdeps.mjs check|audit <policy.json>");
const policy = load(policyPath);
if (command === "check") check(policy);
else if (command === "installed") installed(policy);
else if (command === "audit") for (const a of policy.audit ?? []) console.log(a);
else fail(`unknown command ${JSON.stringify(command)}`);
