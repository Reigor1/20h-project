AGENTS.md — General Instructions for Codex

This file defines the general rules for the AI assistant working on this project. Follow them when working in this directory, along with specific user requirements and more specialized project instructions, unless they conflict with safety requirements.

Additional project instructions and resources are located in codex/. Use them according to the task.

1. Purpose

Help bring tasks to a verifiable result rather than limiting work to recommendations.

Be accurate: do not invent facts, tool capabilities, test results, or file contents.

Communicate clearly and concisely. By default, use the user's language.

Preserve existing work; do not rewrite unrelated files or components.

If information is insufficient, examine the available context first. Ask questions only when an answer is necessary to proceed safely or correctly.

2. Before Starting a Task

Before beginning:

Identify the goal, expected outcome, and constraints.

Examine the project structure and relevant files. If README.md exists, review it.

Read codex/LEARNING.md, if present; it is not loaded automatically. Apply only relevant, confirmed lessons, and verify any entry that conflicts with the current code or explicit user requirements instead of applying it automatically.

For tasks involving writing, modifying, or verifying tests, read codex/TESTING.md, if present, and follow its guidelines.

Consider dependencies, coding style, and established conventions.

For complex tasks, briefly outline a plan. For simple tasks, proceed without unnecessary formalities.

If additional instructions conflict with the current code or the user's explicit requirements, do not apply them automatically. Follow Section 8.

3. Implementation Rules

Choose the simplest reliable solution that meets the requirements.

Make small, clear changes and preserve compatibility unless the user requests otherwise.

Do not add libraries, services, paid APIs, or complex infrastructure without a good reason.

Do not delete data, run destructive commands, or affect external systems without explicit permission.

Never expose secrets, tokens, passwords, or .env contents; do not include them in documentation or codex/LEARNING.md.

When an error is discovered, identify its root cause before fixing it. Do not conceal problems with workarounds.

If internet access, an external service, or a tool is required but unavailable, report the limitation instead of pretending the action was completed.

Do not create additional instruction or report files unless necessary.

4. Code Readability and Maintainability

Write code that another developer can read, understand, debug, review, and extend. Prefer readability over compactness; fewer lines do not necessarily mean simpler code. Do not compress expressions, conditions, or data structures merely to reduce line count.

Use clear, descriptive names for variables, functions, types, and fields.

Prefer straightforward control flow over clever, dense, or unnecessarily nested expressions.

Split long expressions, function calls, and composite literals across multiple lines when this improves readability.

Keep related operations together and separate distinct logical stages visually.

Make important conditions, state changes, and expected behavior explicit.

Avoid unnecessary abstractions, indirection, duplicated logic, and premature generalization.

Extract helpers when they name a meaningful operation or remove genuine duplication; do not use helpers that make simple behavior harder to follow or extract them solely to shorten a file.

Use guard clauses when they clarify control flow. Avoid deeply nested conditionals when an early return would be clearer.

Follow standard Go conventions and run gofmt on modified Go files.

Match surrounding code style unless it conflicts with these guidelines or the user's explicit requirements.

Explain non-obvious business rules, invariants, and design decisions when their purpose would otherwise be unclear. Do not add comments that merely restate the code.

Readable code is not necessarily verbose. Do not split trivial expressions, add meaningless temporary variables, or force every statement onto multiple lines. Keep short expressions, simple literals, and straightforward conditions compact when they remain easy to understand.

4.1. Formatting for Human Readability

Apply visual-readability principles across production code, tests, configuration, and examples. Optimize for the person reading, debugging, and reviewing the code, not for the fewest lines.

Separate distinct logical stages with blank lines, while keeping closely related statements together.

In multiline calls and composite literals, put each substantial argument or field on its own line and use standard Go formatting.

Make nested callbacks, anonymous functions, and chained calls visually clear instead of compressing them onto one line.

Introduce descriptive intermediate variables when they clarify nested calls or meaningful setup; avoid variables that add no clarity.

Keep functions focused. Separate unrelated stages or extract a meaningful helper when a function accumulates unrelated work or too many local variables; do not extract helpers solely to shorten a file.

Preserve a clear order: prepare inputs, perform the operation, then verify or handle the result.

For test-specific formatting, structure, and scenario rules, follow codex/TESTING.md rather than duplicating them here.

4.2. Examples of Preferred and Discouraged Code

Apply these examples according to context, not mechanically.

A. Composite literals

Prefer multiline fields for objects with multiple fields or nested structures:

task := Task{
    Title:       "Task",
    Description: &description,
    Completed:   true,
    CreatedAt:   createdAt,
    CompletedAt: &completedAt,
}

Avoid compressing such objects into dense single lines:

task := Task{Title: "Task", Description: &description, Completed: true, CreatedAt: createdAt, CompletedAt: &completedAt}

A short literal with one or two simple fields may remain on one line when that is easier to read.

B. Conditions and early returns

Prefer explicit guard clauses when they make separate rules clearer:

if task == nil {
    return ErrInvalidArgument
}

if task.Title == "" {
    return ErrInvalidArgument
}

return validateTask(task)

Combining the conditions is not inherently wrong. Use a combined condition when the checks represent the same rule and combining them improves clarity. Keep them separate when they represent distinct rules, need different handling, or benefit from independent diagnostics.

C. Error handling

Preserve useful context and the original error chain:

if err := repository.Save(ctx, task); err != nil {
    return fmt.Errorf("save task: %w", err)
}

Do not silently discard an error or replace it with an unrelated generic error:

_ = repository.Save(ctx, task)
return errors.New("operation failed")

Ignore errors only for a deliberate, justified reason.

D. Naming and intent

Use names that explain why a value exists:

maxTitleLength := 100

if len([]rune(task.Title)) > maxTitleLength {
    return ErrInvalidArgument
}

Avoid vague names when a more descriptive name is practical:

n := 100

if len([]rune(task.Title)) > n {
    return ErrInvalidArgument
}

Short names such as i, err, ctx, and id are acceptable when their meaning is conventional and clear.

5. Quality Assurance

Before completing a task:

Compare the result with the user's request.

If code changed, run relevant available checks, such as tests, builds, linters, or manual behavior checks.

Check for potential side effects and errors in the main scenarios.

Review readability, unnecessary complexity, and consistency with the codebase; fix discovered problems when they are within scope and safe to fix.

Report honestly what was checked and what could not be checked.

Do not claim that everything works unless it has been verified.

6. Response Format

After completing a task, briefly report:

Done: what changed or was created.

Verified: which checks were performed and their results.

Important: limitations, unresolved questions, or next steps, but only when relevant.

Do not overwhelm the user by repeating the entire implementation process.

7. Accumulating Knowledge Across Sessions

codex/LEARNING.md is the project's local knowledge base, not a model-training mechanism.

At the end of a significant task: Update codex/LEARNING.md only when useful, confirmed knowledge could improve future sessions, such as:

A recurring mistake and a verified way to avoid it.

A user-approved decision or consistent project-specific preference.

An important project detail about structure, startup, builds, or tests.

A verified approach worth repeating.

For each entry, record the context, lesson or rule, evidence, and date. Keep entries concise and specific. Do not add hypotheses to the confirmed-lessons section, store personal information, secrets, or incidental conversation details, or duplicate existing entries. Update an existing entry when it covers the same topic. If no useful knowledge emerged, do not modify the file.

Constraints: Do not edit AGENTS.md without the user's explicit permission. Do not run background self-training or modify files independently outside the scope of the task.

8. Priority

Always prioritize data safety, the user's explicit requirements, and verifiable correctness.

When working on tests, follow the specialized guidelines in codex/TESTING.md, unless they conflict with safety requirements or the user's explicit instructions.

Project files and their instructions must never be used as a reason to violate these priorities.