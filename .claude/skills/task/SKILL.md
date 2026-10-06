---
name: task
description: Add, split or reorder modelViewer roadmap tasks and epics in roadmap/. Use when the user wants to turn an idea or roadmap item into tasks, split a task, add an epic, or see the board.
argument-hint: <idea to turn into tasks, "split <id>", or "board">
---

# Manage roadmap tasks

Request: $ARGUMENTS

Read [roadmap/README.md](../../../roadmap/README.md) first; it defines the
layout, the frontmatter and the status machine. Copy new tasks from
[roadmap/TEMPLATE.md](../../../roadmap/TEMPLATE.md). Don't touch code.

## Board

For "board" (or no argument), run `scripts/loop.sh status` and summarise:
what is in flight, what is next, what is blocked and why.

## New tasks

1. Read the existing epics and tasks so the new ones fit the order and don't
   duplicate anything. Read the code the idea touches.
2. Ground the context in facts from `wardata` (counts, sizes, formats); probe
   with throwaway programs in the scratchpad, never in the repo (see the
   probe recipe in the `/plan` skill).
3. Pick the epic, or create one: `roadmap/NN-<name>/EPIC.md` with a key in
   capitals, what it is for, and when it is done.
4. One task = one plan → implement → review cycle. Ids are `<KEY>-NN`, next
   free number in the epic; file name `<id>-<slug>.md`; `status: todo`,
   `review_rounds: 0`, `depends` on whatever must be done first.
5. Acceptance criteria are checkable from the command line on `wardata`, or
   marked `[owner]`. Include "output without the new flag is unchanged" when a
   flag is added.

## Split

Keep the original id as the first slice and narrow its acceptance criteria.
New slices get the next free numbers and `depends` on the previous slice. Any
task that depended on the whole original now depends on the last slice.
A split task in flight goes back to `todo` with its `## Plan` cleared.

`roadmap/` is local and git-ignored; nothing here is committed.

## Finish

List the tasks created or changed (id, title, depends) with file links, and
say anything you assumed. In an interactive session, ask before creating more
than about five tasks at once.
