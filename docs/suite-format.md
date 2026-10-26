# Suite format

A suite is a YAML file describing the tasks an agent will attempt.
Paths inside a task are relative to the directory that contains the
suite file.

    name: core-skills
    version: 1

    defaults:
      timeout: 5m          # Go duration string, inherited by all tasks

    tasks:
      - id: fix-grep       # unique within the suite
        image: python:3.12-alpine
        dir: tasks/fix-grep
        files: files       # optional, copied to /task
        prompt: prompt.md
        golden: solution.sh
        verify:
          - verify.sh
          - test -f /task/artifact.txt   # inline commands also work

## Fields

- `name` (required) — suite name, shows up in reports.
- `version` — schema version, must be an integer.
- `defaults.timeout` — Go duration (`90s`, `2m`, `5m`). Default: `5m`.
- `tasks[].id` (required) — unique task identifier.
- `tasks[].image` (required) — container image. Must provide a POSIX
  shell; the container boots as `sh -c 'sleep ...'` and every
  interaction happens through execs.
- `tasks[].dir` (required) — task directory holding the bundle.
- `tasks[].files` — directory copied to `/task` before the agent runs.
- `tasks[].prompt` (required) — markdown file with the task statement,
  sent verbatim to chat agents.
- `tasks[].golden` (required) — reference solution script, executed by
  the `golden` agent from `/task/.evalrig/solution.sh`.
- `tasks[].verify` (required) — one or more verifiers. Each entry is
  either a script file in the task directory (run as
  `sh /task/.evalrig/<name>`) or an inline shell command. A task passes
  only if every verifier exits `0`.
- `tasks[].timeout` — per-task override of `defaults.timeout`.

## What lands inside the container

    /task
    ├── .evalrig/        # task bundle: prompt, golden, verifiers
    │   ├── prompt.md
    │   ├── solution.sh
    │   ├── verify.sh
    │   └── ...          # any other files in the task directory
    └── ...              # contents of `files`

## Trajectory events

Each (task, agent) attempt writes a JSONL file under
`runs/<timestamp>/traj/<task>-<agent>.jsonl` with the events:

- `task_start` — `{task, agent, image}`
- `llm_request` / `llm_response` — chat agents only
- `exec_command` / `exec_output` — every container command and its
  outcome (output truncated to 8 KiB)
- `verify_result` — `{checks: [{cmd, pass, exit_code, output}]}`
- `task_end` — `{status: pass | fail | error}`
