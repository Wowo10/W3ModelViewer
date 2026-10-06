---
name: plan
description: Investigate a change to modelViewer and propose a step-by-step plan without editing any code. Use when the user asks to plan, scope or think through a feature, refactor or optimization before implementing it, or to plan a roadmap task by id (e.g. CASC-01).
argument-hint: <task id, or what to plan>
---

# Plan a change

Task: $ARGUMENTS

Produce a plan. Do **not** edit, create or delete project code in this skill —
implementation happens later via `/implement`.

**Task mode:** if the argument is a roadmap task id (like `CASC-01`), find its
file with `grep -rl '^id: <id>$' roadmap/` and read it, its `EPIC.md` and
[roadmap/README.md](../../../roadmap/README.md). The task's acceptance criteria
are the goal, and answered Open questions are the owner's decisions. In task
mode the only files you may write are roadmap task files. If the task has no
`base` field, add `base: <git rev-parse HEAD>` to its frontmatter.

## 1. Understand the current code

- Read the files the change touches in full; the codebase is small, so prefer
  reading over guessing.
- Layout:
  - `main.go` — CLI flags, drives the extract pipeline, optionally opens the viewer.
  - `extract/buildinfo` — parses `wardata/.build.info`.
  - `extract/buildConfig` — parses the build config under `wardata/Data/config/`.
  - `extract/idxPackages` — reads `.idx` files from `wardata/Data/data/` and
    (with `-verify`) cross-checks entries against the `data.###` archive headers.
  - `extract/blte`, `extract/encoding` — BLTE decoding and the encoding file.
  - `extract/casc` — `Storage`: lookups and reads by encoding or content key.
  - `viewer` — SDL2 + OpenGL 3.2 + cimgui-go window.
- `wardata/` is real game data (git-ignored). Use it to ground claims with
  numbers — file counts, sizes, entry counts — instead of estimating. Probe
  with throwaway programs in the scratchpad, never in the repo (a stray
  package there breaks `go vet ./...` and stops the loop). A probe that
  imports this module's packages needs its own module:

  ```sh
  mkdir -p <scratchpad>/probe && cd <scratchpad>/probe
  printf 'module probe\n\ngo 1.22\n\nrequire modelView v0.0.0\n\nreplace modelView => <repo root>\n' > go.mod
  # write main.go importing modelView/extract/..., then:
  go mod tidy && go build -o probe . && cd <repo root> && <scratchpad>/probe/probe
  ```

  Run it from the repo root so the relative `wardata/` paths resolve.

## 2. Respect the owner's constraints

- Don't plan changes to parsing logic or error handling unless asked. The code
  panics on bad input and ignores some corner cases on purpose — the owner
  will handle those themselves. If a plan step would change behaviour, call it
  out explicitly.
- Find where the time or complexity actually goes before proposing a fix
  (e.g. syscalls per entry before goroutines).

## 3. Write the plan

1. **Goal** — one or two sentences.
2. **Findings** — what the code does today, with `file:line` links and any
   measurements.
3. **Steps** — numbered, ordered by value, each small enough to implement and
   verify on its own. For each: files touched, what changes, and whether
   observable behaviour changes.
4. **Risks / open questions** — only real ones.
5. **Verification** — how each step and each acceptance criterion will be
   checked (see `/implement`).

**Interactive:** reply in chat, give a recommendation rather than a survey,
and end by asking which step(s) to implement.

**Task mode:** nobody will answer questions. Write the plan into the task's
`## Plan` section (replace any old plan), then set the frontmatter:

- `status: planned` when the plan covers every acceptance criterion.
- Too big for one implement/review cycle: keep this id as the first slice
  (narrow its acceptance criteria), add follow-up tasks with `/task`'s
  conventions (next free number, `depends` on this id), move any `depends` on
  this id that needed the whole thing to the last slice, then `status: planned`.
- A decision only the owner can make, a criterion that is impossible as
  written, or a tool call you need that was denied: write it under
  `## Open questions` (the question with options and your recommendation, or
  the command and why) and set `status: blocked`. Prefer a sensible default,
  recorded in the plan, when the choice is clearly yours to make.

`roadmap/` is local and git-ignored: don't commit task files. This skill
makes no commits.

Finish with a two-line summary of the plan and the new status.
