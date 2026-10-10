---
name: implement
description: Implement an agreed change, plan step or roadmap task in modelViewer and verify it builds and behaves the same (or as intended) on the real wardata. Use when the user asks to implement, apply or do a planned step or task (e.g. CASC-01), or to address review findings.
argument-hint: <task id, or change / plan step to implement>
---

# Implement a change

Task: $ARGUMENTS

If this refers to a plan step ("#2", "step 1"), take it from the plan earlier in
the conversation. If there is no plan and the change is non-trivial, suggest
running `/plan` first. Note `git rev-parse HEAD` before the first edit: that is
the **base** to compare against.

**Task mode:** if the argument is a roadmap task id (like `CASC-01`), find its
file with `grep -rl '^id: <id>$' roadmap/` and read it in full. The base is its
`base` field (set it to `HEAD` if missing). Answered Open questions are the
owner's decisions; follow them. Check `git log --oneline <base>..HEAD` and the
Implementation notes for work already done, and continue from there.

- `status: planned` — implement the `## Plan` against the acceptance criteria.
- `status: changes-requested` — address every **must-fix** finding of the
  latest `### Round N` under `## Review` (and gate output, if that is what it
  holds). Nits are optional. Don't redo work that wasn't flagged.
- Any other status — stop and say the task is not ready to implement.

## Rules

- Do only what was asked. Don't change logic or error handling unless that is
  the task. Keep panics and known corner cases as they are; the owner fixes
  those themselves.
- Match the surrounding code: Go style, naming, comment density, `DEBUG`
  printing pattern.
- New behaviour that changes defaults goes behind a flag in `main.go`, keeping
  the current behaviour as the default unless told otherwise.
- Commit code in small steps following the convention in CLAUDE.md (scope:
  the task id in task mode). Each commit builds. `roadmap/` is git-ignored;
  never commit task files. Never push, amend, rebase, reset, stash or switch
  branches.
- A decision that is the owner's, or a tool call that was denied: don't guess
  or work around it. Interactively, ask. In task mode, commit what you have,
  write under `## Open questions` exactly what you need (the command and why,
  or the question with options and your recommendation), set
  `status: blocked` and stop.

## Verify

Run everything from the repo root. Use the session scratchpad directory for
binaries, probes and output files, not `/tmp` or the repo (for a probe that
imports this module, see the recipe in the `/plan` skill).

1. `gofmt -l .` prints nothing, `go vet ./...` passes, `go build ./...` succeeds.
   When `web/` changes (or once `web/package.json` exists), `npm --prefix web
   ci` if `web/node_modules` is missing, then `npm --prefix web run check`
   passes.
2. Compare the program's output before and after the change:
   - Build the new binary: `go build -o <scratchpad>/new .`
   - Build the old binary from a worktree of the base, leaving the working
     tree alone: `git worktree add --detach <scratchpad>/base <base>`,
     `go -C <scratchpad>/base build -o <scratchpad>/old .`, then
     `git worktree remove --force <scratchpad>/base`.
   - Run both from the repo root against `wardata`, capture stdout+stderr, and
     `diff` them. Stack-trace line numbers may differ; everything else must
     match unless the change is meant to alter output.
   - Exercise any new flag in the new binary.
3. Check each acceptance criterion you can from the command line and note the
   evidence (command and the relevant output lines).
4. For performance work, time both binaries over several runs and report the
   range. If a panic stops the run early, time throwaway copies in the
   scratchpad with that panic disabled — never edit the real code for this.
5. The viewer (`go run .` with no output flag) needs a display; if you can't open it, say it was only
   compile-checked and leave `[owner]` criteria to the owner.

## Report

**Interactive:** tell the user briefly what changed (with file links), the
commits made, what verification ran and its result, and any behaviour change.

**Task mode:** append to `## Implementation notes` a dated entry (`### <date> —
implement` or `— fix round N`) with what changed, the verification and its
result, and evidence per acceptance criterion. Tick the criteria you verified
(`- [x]`). Then set `status: implemented`.
