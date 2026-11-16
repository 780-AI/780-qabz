<!-- VERSION: 0.3 -->
In this directory (expected to be {{project}}/docs/Plans/{{pending|in-progress|done}}_tasks/) we put tasks files (md files).

Each task's name must be lowercase and/or numbers with _.

Each new task gets an ID from `task_counter.json` (in this directory). The name starts with `<ID>_`, for example `002_app_bootstrap.md`. Read `next_task_id`, use it, then increment it by one (3 digits, zero padded) in the same commit that creates the task. Never reuse an ID.

Each task's content must have:
- A heading
- A small text with status "pending", "in-progress" or "done".
- A subheading named "Description"
- A short paragraph describing the task
- A subheading named "Definition of Done"
- A short bullet list of expectation. Absolutely necessary expectations. Do not be verbose.

When a task gets started, put it in in-progress_tasks, and when done, put it in done_tasks.

Template file must only be in pending_tasks.
