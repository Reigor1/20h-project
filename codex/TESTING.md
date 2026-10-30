TESTING.md — Testing Guidelines

1. Purpose and priorities

Write clear, reliable, maintainable tests that verify application behavior and detect regressions.

Prioritize, in order:

Correctness of assertions.

Readability and maintainability.

Independence and reproducibility.

Ease of extending tests.

Gradual improvement of meaningful coverage.

Do not maximize coverage percentages at the expense of useful tests. Do not modify production code solely to make tests pass. If an implementation defect is discovered, report it and separate its fix from the test-writing task.

2. Before writing or changing tests

Study the implementation, its dependencies, and the behavior or contract under test.

Review existing tests, project conventions, and relevant internal/testutil helpers.

Check for duplicated helper functionality across packages.

Identify the main scenarios and risks before implementing tests.

Follow actual business rules; do not invent extra constraints to increase coverage.

Shared test utilities

For every testing task, even when the task is unrelated to helpers:

Inspect internal/testutil and reuse suitable existing helpers.

Extract genuinely shared functionality into internal/testutil when this reduces meaningful duplication.

Keep helpers local when sharing would obscure setup, add unnecessary abstraction, or couple unrelated tests.

When extracting a helper, update its callers and run the affected tests.

3. Test structure and scenario selection

Use Go's standard testing package unless an additional library solves a specific problem that the standard library does not adequately address.

Prefer table-driven tests when several meaningful scenarios share execution and assertion logic. Use a slice of structs, descriptive scenario names, and t.Run().

Keep each scenario's inputs and expected results together so adding a case is straightforward.

Use separate Test... functions for logically independent behaviors; keep unrelated methods or behaviors separate.

Do not use table-driven tests mechanically: a regular test is often clearer for one simple scenario.

Cover normal, failure, boundary, and corner cases

Choose test cases based on the contract and risk. Consider, when relevant:

Valid and invalid inputs, including the expected error type or identity.

Boundary values and transitions at or around limits.

Empty values and states, nil, missing fields, and zero values when meaningful under the contract.

Malformed input, duplicate or conflicting data, and unusual combinations of fields.

Error paths, state changes, and unexpected side effects.

Preservation of original state after an error when required by the contract.

Repeated calls and relevant ordering or lifecycle cases when behavior may differ across calls.

Partial failures and cleanup behavior when operations can leave state behind.

Do not overlook corner cases. Prioritize plausible defects and representative high-risk edge conditions; do not enumerate every imaginable input, add redundant cases, or invent business rules.

Do not merely assert that an error occurred. Verify its type or identity when relevant, using errors.Is() or errors.As() for wrapped errors. Include useful actual and expected values in assertion messages. Specify expected results independently; do not duplicate production logic to calculate them.

4. Test-specific readability and formatting

Follow the general readability and formatting rules in AGENTS.md. These additional conventions apply specifically to tests:

Format table cases as multiline struct literals, with each field on its own line. Use multiline nested literals when they contain multiple fields or would otherwise be hard to scan. Keep formatting consistent within a table.

Name scenarios after the behavior being verified, not with vague labels such as case 1.

Keep setup, execution, and assertions visually distinct. Split long test calls and fixtures when this helps a reviewer understand the operation and its inputs.

Avoid hiding important setup, expected results, or conditions behind layers of helpers. Explain non-obvious test assumptions or business rules, but do not add comments that restate the code.

Run gofmt on modified Go files; do not rely on it to fix overly dense test code.

5. Test data and helpers

Use the minimum data needed to make the scenario clear. Keep scenario-specific inputs, changed fields, and expected values in the relevant test case.

Prefer direct construction when it is easy to understand. Small helpers that create valid defaults are acceptable when setup requires many fields; name them by purpose and call t.Helper() inside test helpers.

Prevent helpers from hiding important conditions or introducing shared mutable state. Avoid universal testing frameworks and complex builders when a simple function is sufficient.

Keep test data local to the tests that use it. Do not create package-level values used by only one test.

For a fixed value reused within a test, use a descriptive local variable or const when appropriate. Keep simple one-use literals inline. Use package-level constants only for canonical values intentionally shared across tests; avoid package-level mutable test data.

6. Test independence and external dependencies

Tests should be reproducible and, whenever possible, independent of the external environment.

For unit tests:

Do not connect to PostgreSQL or other external services unless necessary.

Do not use real HTTP requests or Docker to test pure business logic.

Do not depend on test execution order.

Control dependencies on time, randomness, and mutable global state; use fixed values when possible.

Use t.Parallel() only when tests and their dependencies are genuinely independent.

Avoid shared mutable objects that let one test affect another.

External dependencies are appropriate for integration and end-to-end tests, but their setup and lifecycle must be explicit and manageable. Do not use mocks as a substitute for testing real queries, migrations, transactions, or other behavior that requires the actual dependency.

7. File placement and test types

Place tests alongside source code in *_test.go files unless the project has an established alternative for a test category.

Follow existing package naming conventions and use the Test... naming convention.

Choose the testing level according to the behavior:

Unit: test a component in isolation.

Integration: test interactions between components and their dependencies.

End-to-end: test a complete application workflow through an external interface.

Follow the project's established test structure. Add infrastructure only when there is a clear reason.

8. Verification and failure handling

After writing or changing tests:

Run gofmt on modified Go files.

Run tests for the target package or feature.

Run go test ./... if the project state and environment allow it.

Review assertions to confirm they verify the intended behavior, including relevant corner cases.

Check independence, readability, naming, formatting, and ease of extending the tests.

Report which checks were run, which passed, and which failed or could not be run. Never claim a check passed unless it actually did.

If a test fails, investigate the cause before changing the test or production code. Do not weaken a valid assertion simply to get a passing result. Distinguish a test defect, an implementation defect, an environment problem, and an incorrect expectation before deciding what to change.

9. Gradually improving test coverage

Do not try to implement every possible scenario in one iteration. Establish a simple, stable testing structure, then expand it incrementally based on risk and meaningful business logic.

For internal/core/domain, prioritize validation, calculations, patch application, state transitions, and constructors when they contain meaningful logic.

Coverage percentage is a diagnostic signal, not a standalone measure of test quality. Prefer tests that can reveal regressions and remain understandable as the code evolves.