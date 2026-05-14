Convert the spec into a canonical SWIM markdown plan using txtstore.

Input spec:
<path-to-spec.md>

Output plan:
docs/superpowers/plans/<yyyy-mm-dd-short-name>.md

Required workflow:
1. Read the spec and identify implementation tasks, dependencies, target files, doc refs, and FP refs.
2. Build a structured SWIM plan payload with:
   - title
   - meta
   - plan
   - doc_index
   - fp_index
   - tasks
   - units
3. Use `txtstore_write_swim_plan` MCP if available, otherwise use:
   `txtstore write-swim-plan <output-plan.md> <payload.json>`
4. Do not hand-edit the SWIM markdown tables if structured payload generation is possible.
5. Ensure every unit has a stable `unit_id`, `task_id`, `title`, `kind`, `wave`, `plan_line`, `depends_on`, `doc_refs`, and `fp_refs`.
6. Validate dependency ordering:
   - no cycles
   - dependencies point to existing units
   - waves respect dependencies
7. After writing the plan, run the existing plan-to-waveplan conversion/validation commands used in this repo, if available.
8. Report:
   - output plan path
   - task/unit count
   - dependency waves
   - validation command/results

