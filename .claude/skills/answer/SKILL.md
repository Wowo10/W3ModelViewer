---
name: answer
description: Answer a blocked modelViewer roadmap task's Open questions as multiple choice, write the answers into the task file and unblock it for the loop. Use when the owner wants to answer or unblock a blocked task (e.g. ANIM-02), or asks what is blocked.
argument-hint: [task id; default: every blocked task]
---

# Answer a blocked task

Task: $ARGUMENTS

Read [roadmap/README.md](../../../roadmap/README.md) for the task format and
the status machine. This skill is interactive: it asks the owner and never
picks an answer itself. Don't touch code.

## 1. Find the task

With an id, use `roadmap/*/<id>-*.md`. Without one, list the tasks with
`status: blocked` (`grep -l '^status: blocked' roadmap/*/*.md`): none → say
so and stop; several → answer them one after another, in path order.

## 2. Understand what it is blocked on

Read the whole task file: Goal, Acceptance criteria, the Plan's findings
that the question refers to, the latest `### Round N` under Review, and Open
questions. A question is open when no `> Answer:` line follows it. Classify
each open question:

- **Decision**: a choice with options (usually lettered, one recommended).
- **Denied command**: a tool call the agent needed and was refused.
- **Review limit**: the loop's note under Review that the task ran out of
  review rounds (`_The loop blocked this task after N review rounds…_`).

Check numbers or files the question rests on only when it is unclear what
an option would mean in practice; don't redo the agent's investigation.

## 3. Ask

Ask everything for one task in a single `AskUserQuestion` call (up to 4
questions; more → several calls). Per question:

- `question`: one sentence the owner can answer without opening the file,
  with the key number (e.g. "2,195 classic vertices have 5–8 nodes; how many
  influences per vertex?"). `header`: the task id plus a short topic.
- `options`: the task's options, the recommended one first with
  "(Recommended)" in its label. `description`: what changes for the code
  and the data if it is chosen, in one or two sentences, taken from the
  task. Use `preview` for code or layout differences when they help compare.
- **Denied command** options: allow it (say the permission rule you would
  add to `.claude/settings.json`), do it another way (the agent's
  alternative, if it gave one), or skip what needed it.
- **Review limit** options: more rounds (unblock to `changes-requested`),
  accept as is (unblock to `approved`), replan (unblock to `todo`).

"Other" is always available for a free answer; take it as given.

## 4. Write the answers

Under each question, write the answer in the owner's words where they gave
any, otherwise the option itself:

```
  > Answer: A — up to 8 per vertex. <owner's note, if any>
```

Keep the question text; don't edit the Plan or Implementation notes. If an
answer contradicts an acceptance criterion or the Out of scope list, change
that line to match and end it with `(changed by Open questions)`, so review
checks the decision rather than the old wording. For an allowed command, add
the rule to `.claude/settings.json` `permissions.allow` (load the
`update-config` skill) and say so in the answer.

## 5. Unblock

Run `scripts/loop.sh unblock <id>` (or `unblock <id> <status>` when the
answer chose one, e.g. for a review limit). It puts the task back to its
`resume` status, so the blocked phase runs again with the answers; a waiting
loop picks it up within a minute.

## Finish

Per task: the answers written, any criterion changed, and the status it went
back to. `roadmap/` is git-ignored; nothing here is committed unless a
settings change was made.
