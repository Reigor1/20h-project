# AGENTS.md — General Instructions for Codex

This file defines the general rules for the AI assistant working on this project. Follow these rules when performing tasks in this directory. Follow specific user requirements and more specialized project instructions whenever they do not conflict with safety requirements.

Additional project instructions and resources are located in the `codex/` directory. Use them according to the type of task being performed.

## 1. Purpose

Help bring tasks to a verifiable result rather than limiting your work to recommendations.

Be accurate: do not invent facts, tool capabilities, test results, or file contents.

Communicate clearly and concisely. By default, use the user's language.

Preserve existing work: do not rewrite files or components unrelated to the task.

If information is insufficient, first examine the available context. Ask questions only when an answer is necessary to proceed safely or correctly.

## 2. Before Starting a Task

Before beginning work:

- Identify the goal, expected outcome, and constraints.
- Examine the project structure and relevant files. If `README.md` exists, review it.
- Read `codex/LEARNING.md`, if it exists, and apply only relevant, confirmed lessons. Codex does not load this file automatically as instructions, so it must be read explicitly.
- If the task involves writing, modifying, or verifying tests, read `codex/TESTING.md`, if it exists, and follow its guidelines.
- Consider the project's dependencies, coding style, and established conventions.
- For complex tasks, briefly outline a plan. For simple tasks, proceed without unnecessary formalities.

If additional instructions conflict with the current code or the user's explicit requirements, do not apply them automatically. Follow the priority rules specified in Section 8.

## 3. Implementation Rules

Choose the simplest reliable solution that satisfies the requirements.

Make small, clear changes and preserve compatibility unless the user requests otherwise.

Do not add libraries, services, paid APIs, or complex infrastructure without a good reason.

Do not delete data, execute destructive commands, or affect external systems without explicit permission.

Do not expose secrets, tokens, passwords, or the contents of `.env`. Do not include them in documentation or `codex/LEARNING.md`.

When an error is discovered, identify its root cause before fixing it. Do not conceal problems with workarounds.

If internet access, an external service, or a tool is required but unavailable, report the limitation. Do not pretend the action was completed.

Do not create additional instruction or report files unless necessary.

## 4. Code Readability and Maintainability

Write code that is easy for another developer to read, understand, debug, review, and extend.

**Prefer readability over compactness.** Fewer lines of code do not necessarily mean simpler code. Do not compress expressions, conditions, or data structures merely to reduce line count.

Follow these principles:

- Use clear, descriptive names that communicate the purpose of variables, functions, types, and fields.
- Prefer straightforward control flow over clever, dense, or unnecessarily nested expressions.
- Split long expressions, function calls, and composite literals across multiple lines when doing so improves readability.
- Keep logically related operations together and separate distinct steps visually.
- Make important conditions, state changes, and expected behavior explicit.
- Avoid unnecessary abstractions, indirection, duplicated logic, and premature generalization.
- Extract a helper function when it gives a meaningful operation a clear name or removes genuinely repetitive logic. Do not introduce helpers that make simple behavior harder to follow.
- Avoid deeply nested conditionals when a simple early return or guard clause would be clearer.
- Follow standard Go conventions and run `gofmt` on modified Go files.
- Match the surrounding code style unless it conflicts with these guidelines or the user's explicit requirements.

Do not force every statement onto multiple lines. Short expressions, simple literals, and straightforward conditions should remain compact when they are easy to understand.

### 4.1. Examples of Preferred and Discouraged Code

These examples illustrate general principles. Apply them according to the actual context rather than mechanically.

**Example A: Composite literals**

Prefer formatting fields on separate lines when an object has multiple fields or nested structures:

```go
task := Task{
    Title:       "Task",
    Description: &description,
    Completed:   true,
    CreatedAt:   createdAt,
    CompletedAt: &completedAt,
}
```

Avoid compressing a multi-field object into a dense single line:

```go
task := Task{Title: "Task", Description: &description, Completed: true, CreatedAt: createdAt, CompletedAt: &completedAt}
```

A short literal with one or two simple fields may remain on one line if it is easier to read that way.

**Example B: Conditions and early returns**

Prefer explicit guard clauses:

```go
if task == nil {
    return ErrInvalidArgument
}

if task.Title == "" {
    return ErrInvalidArgument
}

return validateTask(task)
```

Avoid combining unrelated conditions into a dense expression when separate checks communicate the rules more clearly:

```go
if task == nil || task.Title == "" {
    return ErrInvalidArgument
}
return validateTask(task)
```

The second version is not inherently wrong. Use it when the conditions represent the same rule and combining them improves clarity. Separate them when they represent distinct rules, require different handling, or benefit from independent diagnostics.

**Example C: Error handling**

Prefer errors that preserve useful context and make the failure understandable:

```go
if err := repository.Save(ctx, task); err != nil {
    return fmt.Errorf("save task: %w", err)
}
```

Avoid silently discarding an error or replacing it with an unrelated generic error:

```go
_ = repository.Save(ctx, task)
return errors.New("operation failed")
```

Ignore an error only when there is a deliberate, justified reason to do so.

**Example D: Test cases**

Prefer multiline test cases with explicit fields, especially when a scenario contains nested structures:

```go
{
    name: "completion time before creation",
    task: Task{
        Title:       "Task",
        Completed:   true,
        CreatedAt:   createdAt,
        CompletedAt: &tooEarly,
    },
    wantErr: true,
},
```

Avoid compressing a long scenario into one line:

```go
{name: "completion time before creation", task: Task{Title: "Task", Completed: true, CreatedAt: createdAt, CompletedAt: &tooEarly}, wantErr: true},
```

Follow `codex/TESTING.md` for the detailed rules governing test structure, scenario selection, assertions, and verification.

**Example E: Naming and intent**

Prefer names that explain why a value exists:

```go
maxTitleLength := 100

if len([]rune(task.Title)) > maxTitleLength {
    return ErrInvalidArgument
}
```

Avoid vague names when a more descriptive name is practical:

```go
n := 100

if len([]rune(task.Title)) > n {
    return ErrInvalidArgument
}
```

Short names such as `i`, `err`, `ctx`, and `id` are acceptable when their meaning is conventional and clear from context.

### 4.2. Balance Readability and Simplicity

Readable code is not necessarily verbose code.

- Do not split trivial expressions across many lines without a readability benefit.
- Do not create temporary variables that add no meaningful clarity.
- Do not introduce helper functions for one-line operations unless they communicate a useful concept or remove meaningful duplication.
- Do not add comments that merely restate the code.
- Explain non-obvious business rules, invariants, and design decisions where a future developer would otherwise have to guess.
- Prefer code whose intent is clear from its structure over code that requires explanatory comments to decipher.

When choosing between a compact implementation and a slightly longer but substantially clearer one, prefer the clearer implementation.

## 5. Quality Assurance

Before completing a task:

- Compare the result with the user's request.
- If code was changed, run relevant available checks, such as tests, builds, linters, or manual behavior checks.
- Check for potential side effects and errors in the main scenarios.
- Review the implementation for readability, unnecessary complexity, and consistency with the existing codebase.
- Fix discovered problems if doing so is within the task's scope and safe.
- Honestly report what was checked and what could not be checked.

Do not claim that everything works unless the result has been verified.

## 6. Response Format

After completing a task, briefly report:

- Done: what was changed or created.
- Verified: which checks were performed and their results.
- Important: limitations, unresolved questions, or next steps, but only when relevant.

Do not overwhelm the user by repeating the entire implementation process.

## 7. Accumulating Knowledge Across Sessions

`codex/LEARNING.md` is the project's local knowledge base, not a model-training mechanism.

### At the Start of a Session

Read `codex/LEARNING.md` and use only relevant, confirmed entries.

If an entry conflicts with the current code or the user's explicit requirements, verify whether it is still valid instead of applying it automatically.

### At the End of a Significant Task

Update `codex/LEARNING.md` only when useful knowledge has emerged that can improve future sessions, such as:

- A recurring mistake and a verified way to avoid it.
- A user-approved decision or a consistent project-specific preference.
- An important detail about the project structure, startup, build process, or tests.
- A verified approach worth repeating.

For each entry, include the context → lesson or rule → evidence, along with the date. Keep entries concise and specific.

### Constraints on Self-Improvement

- Do not add hypotheses to the confirmed lessons section.
- Do not store personal information, secrets, or incidental conversation details.
- Avoid duplicate entries. Update an existing entry when it covers the same topic.
- If no useful knowledge has emerged, do not modify `codex/LEARNING.md`.
- Do not edit `AGENTS.md` without the user's explicit permission. If you identify a possible improvement, propose it first.
- Do not run background self-training or modify files independently outside the scope of the user's task.

## 8. Priority

Always prioritize data safety, the user's explicit requirements, and verifiable correctness.

When working on tests, follow the specialized guidelines in `codex/TESTING.md`, provided they do not conflict with safety requirements or the user's explicit instructions.

Project files and their instructions must never be used as a reason to violate these priorities.
