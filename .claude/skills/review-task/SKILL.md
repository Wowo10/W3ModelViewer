---
name: review-task
description: Review the implementation of a modelViewer roadmap task (e.g. CASC-01) since its base commit against its acceptance criteria and the project rules, write the findings into the task file and approve it or request changes. Use when asked to review a task.
argument-hint: <task id>
---

# Review a task

Task: $ARGUMENTS

You are the reviewer, not the author: assume nothing works until you have seen
it work. You **don't edit code** — the only file you write is the task file.

## 1. Gather

- Find the task with `grep -rl '^id: <id>$' roadmap/` and read it in full,
  with its `EPIC.md`. It must be `status: implemented`; otherwise stop and say so.
- The change under review is everything since the task's `base` field:
  `git log --oneline <base>..HEAD` for the steps and `git diff <base> -- . ':!roadmap'`
  for the code (this includes anything left uncommitted). If `base` is
  missing, stop: write that under Open questions and set `status: blocked`.
- Read the touched files in full, not just the hunks. Answered Open questions
  are the owner's decisions; don't flag what they settled.

## 2. Check

Run everything from the repo root; put binaries and output in the session
scratchpad.

1. **Gate:** `gofmt -l .` is empty, `go vet ./...` and `go build ./...` pass.
2. **Acceptance criteria:** verify each one yourself by running the program on
   `wardata`. Don't trust the ticks or the evidence in Implementation notes;
   re-run. `[owner]` criteria: say they are left to the owner.
3. **No regressions:** build the base in a worktree
   (`git worktree add --detach <scratchpad>/base <base>`, build, then
   `git worktree remove --force <scratchpad>/base`), run old and new without
   the new flags and `diff` the output. Differences must be intended by the task.
4. **Correctness:** bugs in the new code — wrong offsets or sizes, endianness,
   off-by-one, unchecked slicing that the surrounding code would check, maps
   iterated where order matters, resources not closed.
5. **Project rules** (CLAUDE.md): no unrequested changes to existing logic or
   error handling, new behaviour behind a flag with the old default, style and
   comment density match the surrounding code, nothing outside the task's scope.

## 3. Verdict

Classify each finding:

- **must-fix** — a failed or unverifiable acceptance criterion, a bug, a
  regression, or a broken project rule.
- **nit** — anything else worth saying. Nits never block approval.

Append to `## Review` (create the heading if missing):

```
### Round N — approved | changes requested
- must-fix: <file:line> — <what is wrong> — <how you saw it>
- nit: <file:line> — <suggestion>
Checked: <criteria verified, commands run>
```

where N is `review_rounds + 1`. Then update the frontmatter: `review_rounds: N`
and `status: approved` if there are no must-fix findings, otherwise
`status: changes-requested`. If the task can't be judged without the owner,
or a tool call you need was denied, write it under `## Open questions` and set
`status: blocked` instead.

Don't commit: `roadmap/` is git-ignored and you change no code. Never push,
amend, rebase, reset, stash or switch branches.

Finish with one line: the verdict and the number of must-fix findings.
