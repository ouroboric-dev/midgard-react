#!/bin/bash
# Sourced by the checks of a web leaf, not run (change typed-boundaries, design section 4): it makes the check
# write a typed result, ygg.check/v1, through ygg-check-result, the converter of the work image. Odin reads that
# result and makes one finding per failure. The result goes where YGG_CHECK_RESULT says; with it unset (a local
# run, a command Odin runs untyped) nothing here does anything, and the script is as it was.
#
# A check sources this file, traps its EXIT with ygg_finish, and tells this file which step it is in:
#   ygg_stage KIND MESSAGE [LOGFILE [lines]]   the step has no machine output; if it fails, one failure of KIND says MESSAGE
#                                              and, with LOGFILE, ends with the tail of that file (with "lines", each
#                                              line of the file is a failure of its own)
#   ygg_tool FORMAT INPUT...                   the step writes machine output to the INPUT files, which the converter reads
#   ygg_logged KIND MESSAGE CMD...             runs CMD like ygg_stage and ygg_logged's log: output on the log as always
#   ygg_protected                              runs web-protected.sh; its refusals become the policy failures
# The step that was running when the script ended is the one the result is about: the scripts stop at the first
# problem. The tools' machine output goes to files in the folder ygg_out, never to the log, and never merged with
# stderr. ygg_finish takes the script's exit code; it never changes it, except that a converter that fails makes a
# passing script fail (a realm failure, not a finding).
ygg_t0="$(date +%s%3N)"
ygg_out=""
ygg_fmt=none ygg_inputs=() ygg_kind=other ygg_msg="" ygg_log="" ygg_split=""
ygg_leaf="" ygg_root=""
ygg_on() { [ -n "${YGG_CHECK_RESULT:-}" ]; }
if ygg_on; then ygg_out="$(mktemp -d "${TMPDIR:-/tmp}/ygg-result.XXXXXX")"; fi

ygg_stage() { # kind message [logfile [lines]]
  ygg_fmt=none ygg_inputs=() ygg_kind="$1" ygg_msg="$2" ygg_log="${3:-}" ygg_split="${4:-}"
}

ygg_tool() { # format input...
  ygg_fmt="$1" ygg_inputs=("${@:2}") ygg_kind=other ygg_msg="" ygg_log="" ygg_split=""
  ygg_root="${ygg_leaf:-$PWD}"
}

ygg_logged() { # kind message cmd...: output goes to the log and to a file the failure's message ends with
  local kind="$1" msg="$2" rc=0 f
  shift 2
  if ! ygg_on; then "$@"; return; fi
  f="$(mktemp "$ygg_out/log.XXXXXX")"
  ygg_stage "$kind" "$msg" "$f"
  "$@" 2>&1 | tee "$f" || rc="${PIPESTATUS[0]}"
  return "$rc"
}

# ygg_lines is ygg_logged with a failure for each line of the output.
ygg_lines() { # kind message cmd...
  local kind="$1" msg="$2" rc=0 f
  shift 2
  if ! ygg_on; then "$@"; return; fi
  f="$(mktemp "$ygg_out/log.XXXXXX")"
  ygg_stage "$kind" "$msg" "$f" lines
  "$@" 2>&1 | tee "$f" || rc="${PIPESTATUS[0]}"
  return "$rc"
}

# ygg_protected runs the first step of every rung. web-protected.sh writes what it refuses, a line each, to the file
# YGG_POLICY_MESSAGES names, and does not write a result of its own when it is called from here.
ygg_protected() {
  local here rc=0
  here="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
  if ! ygg_on; then "$here/web-protected.sh"; return; fi
  : >"$ygg_out/policy"
  ygg_stage policy "protected: the check could not run" "$ygg_out/policy" lines
  YGG_CHECK_RESULT="" YGG_POLICY_MESSAGES="$ygg_out/policy" "$here/web-protected.sh" || rc=$?
  return "$rc"
}

# The audits write their machine output to a file; on a failure the log gets the readable report too.
ygg_npm_audit() {
  local rc=0
  if ! ygg_on; then npm audit --audit-level=high; return; fi
  ygg_tool npm-audit-json "$ygg_out/audit.json"
  npm audit --audit-level=high --json >"$ygg_out/audit.json" || rc=$?
  [ "$rc" -eq 0 ] || npm audit --audit-level=high || true
  return "$rc"
}
ygg_osv() {
  local rc=0
  if ! ygg_on; then osv-scanner scan source -L package-lock.json; return; fi
  ygg_tool osv-json "$ygg_out/osv.json"
  osv-scanner scan source -L package-lock.json --format json >"$ygg_out/osv.json" || rc=$?
  [ "$rc" -eq 0 ] || osv-scanner scan source -L package-lock.json || true
  return "$rc"
}

# ygg_oasdiff OLD NEW runs oasdiff breaking with --fail-on ERR. With a result to write its JSON goes to a file and, on a
# failure, the log gets the readable report too.
ygg_oasdiff() {
  local rc=0
  if ! ygg_on; then oasdiff breaking "$1" "$2" --fail-on ERR; return; fi
  oasdiff breaking "$1" "$2" --format json --fail-on ERR >"$ygg_out/oasdiff.json" || rc=$?
  [ "$rc" -eq 0 ] || oasdiff breaking "$1" "$2" --fail-on ERR || true
  return "$rc"
}

# ygg_result FORMAT INPUT... writes the result of a script that ended with the exit code in ygg_code, from tool output
# in the INPUT files (format none: no input, the failures of ygg_pairs). Extra converter flags are in ygg_extra.
ygg_result() {
  local fmt="$1" args=() ms
  shift
  ms=$(($(date +%s%3N) - ygg_t0))
  args=(--check "$(basename "$0" .sh)" --exit "$ygg_code" --duration-ms "$ms" --format "$fmt")
  [ "$ygg_code" -ne 124 ] || args+=(--timed-out)
  local f
  for f in "$@"; do args+=(--input "$f"); done
  ygg-check-result "${args[@]}" "${ygg_extra[@]}"
}

ygg_code=0 ygg_extra=()
ygg_finish() { # exit code of the script
  ygg_on || return 0
  ygg_code="$1" ygg_extra=()
  local fmt="$ygg_fmt" inputs=("${ygg_inputs[@]}") f line n=0 body rc=0
  if [ "$fmt" != none ]; then
    # A tool that died has no output to read: a non-zero exit with an empty file is a failure of the step, said plainly.
    for f in "${inputs[@]}"; do
      if [ "$ygg_code" -ne 0 ] && [ ! -s "$f" ]; then
        fmt=none inputs=() ygg_kind=other ygg_msg="a tool of the check failed (exit $ygg_code) and wrote no machine output; the log has what it printed" ygg_log="" ygg_split=""
      fi
    done
  fi
  if [ "$fmt" = none ]; then
    if [ "$ygg_code" -ne 0 ]; then
      if [ -n "$ygg_split" ] && [ -s "$ygg_log" ]; then
        while IFS= read -r line && [ "$n" -lt 100 ]; do
          [ -n "$line" ] || continue
          ygg_extra+=(--kind "$ygg_kind" --message "$line")
          n=$((n + 1))
        done <"$ygg_log"
      fi
      if [ "$n" -eq 0 ]; then
        body="${ygg_msg:-the check failed} (exit $ygg_code)"
        if [ -n "$ygg_log" ] && [ -s "$ygg_log" ]; then body+=$'\n'"$(tail -n 30 "$ygg_log")"; fi
        ygg_extra+=(--kind "$ygg_kind" --message "$body")
      fi
    fi
  else
    ygg_extra+=(--root "$ygg_root")
    [ "$fmt" != npm-audit-json ] || ygg_extra+=(--level high)
  fi
  ygg_result "$fmt" "${inputs[@]}" || rc=$?
  rm -rf -- "$ygg_out"
  if [ "$rc" -ne 0 ]; then
    echo "ygg-check-result failed (exit $rc): the check's result was not written" >&2
    [ "$ygg_code" -ne 0 ] || exit "$rc"
  fi
}
