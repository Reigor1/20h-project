TESTING.md — Testing Guidelines
1. Purpose
Write clear, reliable, and maintainable tests that verify application behavior and help detect regressions.

Priorities:
1. Correctness of assertions.
2. Readability and maintainability of test code.
3. Independence and reproducibility of results.
4. Ease of extending existing tests.
5. Gradual improvement of meaningful test coverage.
Prefer readable code over compact code. Do not optimize tests for fewer lines or characters at the expense of clarity.

Do not pursue maximum coverage percentages at the expense of useful tests.
2. Before Writing Tests
Before starting:
- Study the implementation being tested and its dependencies.
- Check existing tests and the project's conventions.
- Identify the behavior and contract that need to be verified.
- Follow actual business rules without inventing additional constraints.
- Outline the main test scenarios before implementing them.
Do not modify production code solely to make tests pass. If an implementation defect is discovered, separate its fix from the test-writing task and explain the reason.
3. Test Structure
Use Go's standard testing package unless additional libraries are genuinely necessary.

Prefer table-driven testing when a method has multiple meaningful input scenarios that can share the same test logic.
- Create a separate Test... function for each logically independent behavior.
- Define test scenarios in a slice of structs.
- Use t.Run() to execute named scenarios.
- Keep each scenario's input data and expected results together.
- Make it possible to add a scenario without duplicating the test execution and assertion logic.
- Keep unrelated behaviors in separate test functions.
Do not use table-driven tests mechanically. For simple behavior with only one meaningful scenario, a regular test may be clearer.
4. Code Readability and Formatting
Test code must be easy to read, understand, review, debug, and extend.
4.1. Table-driven test cases
- Format each test case as a multiline struct literal.
- Put each field on its own line.
- Format nested struct literals across multiple lines when they contain multiple fields or would otherwise be difficult to read.
- Use consistent formatting across all entries in the same test table.
- Avoid compressing long test cases into single lines.
- Give scenarios descriptive names that explain the behavior being tested.
Prefer:
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
Avoid:
{name: "completion time before creation", task: Task{Title: "Task", Completed: true, CreatedAt: createdAt, CompletedAt: &tooEarly}, wantErr: true},
4.2. General formatting rules
- Prefer straightforward control flow over dense or unnecessarily nested expressions.
- Split long function calls, composite literals, and conditions across multiple lines when doing so improves readability.
- Keep test setup, execution, and assertions visually distinct.
- Use intermediate variables when they clarify the meaning of an expression or expected value.
- Prefer clear, descriptive names over vague names when the meaning is not obvious from context.
- Avoid unnecessary abstractions, helper layers, and excessive comments.
- Do not add comments that merely repeat what the code already says.
- Run gofmt on modified Go files, but do not rely on it to improve the structure of overly dense code.
Short expressions, simple literals, and straightforward conditions may remain on one line when they are easy to understand.
4.3. Examples of readable code
Prefer explicit object construction:
task := Task{
    Title:       "Task",
    Description: &description,
    Completed:   true,
    CreatedAt:   createdAt,
    CompletedAt: &completedAt,
}
Avoid compressing a multi-field object into a single line:
task := Task{Title: "Task", Description: &description, Completed: true, CreatedAt: createdAt, CompletedAt: &completedAt}
Prefer clear error handling:
if err := repository.Save(ctx, task); err != nil {
    return fmt.Errorf("save task: %w", err)
}
Avoid silently discarding errors without a justified reason:
_ = repository.Save(ctx, task)
return nil
Use guard clauses when they make control flow clearer:
if task == nil {
    return ErrInvalidArgument
}
if task.Title == "" {
    return ErrInvalidArgument
}
return validateTask(task)
Combining conditions is acceptable when they represent the same rule and remain easy to understand. Do not split simple expressions into multiple branches without a meaningful benefit.

Use descriptive variable names when they clarify intent:
maxTitleLength := 100
if len([]rune(task.Title)) > maxTitleLength {
    return ErrInvalidArgument
}
Short names such as i, err, ctx, and id are acceptable when their meaning is conventional and clear from context.
4.4. Balance readability and simplicity
Readable code is not necessarily verbose code.
- Do not split every statement across multiple lines.
- Do not create temporary variables that add no meaningful clarity.
- Do not introduce helper functions for trivial operations unless they communicate a useful concept or remove meaningful duplication.
- Do not add comments merely to explain obvious operations.
- Explain non-obvious business rules, invariants, and decisions when their purpose would otherwise be unclear.
When choosing between compact code and a slightly longer but substantially clearer implementation, prefer the clearer implementation.
5. Test Data Setup
If creating objects requires setting many fields, small helper functions that create valid default objects are allowed.

For example, a helper can create a valid task, while an individual scenario overrides only the fields it needs.

Rules:
- Use the minimum test data necessary.
- Make modified fields and important values explicit in each scenario.
- Name helper functions according to their purpose.
- Call t.Helper() inside test helper functions.
- Prefer direct test data over helpers when direct construction is easier to understand.
- Do not hide expected results or important conditions behind multiple layers of abstraction.
- Ensure that helpers do not introduce shared mutable state between tests.
Do not create universal testing frameworks or complex builders when a simple function is sufficient.
Test values, variables, and constants
- Keep test data local to the test that uses it. Do not declare a package-level variable or constant for a value used by only one test.
- When a fixed test value is used more than once within a test—for example, a log message passed to the logger and then checked in the output—give it a descriptive local name. Prefer a local const for fixed values when possible; use a regular variable when the value is computed or needs to change.
- Use names that explain the value's role, such as logMessage or requestID, rather than vague names such as value. Conventional names such as got and want are fine when their meaning is clear.
- Keep simple values inline when they are used only once and extracting them would not improve readability. Do not introduce variables merely to avoid a short literal.
- Use package-level constants only when the same canonical test value is intentionally shared by multiple tests. Avoid package-level mutable test data and shared mutable state.
- In table-driven tests, keep scenario-specific inputs and expected values in the corresponding test-case struct so each scenario remains easy to understand and change independently.
For example, when checking that a logger writes the exact message supplied to it, reuse one clearly named local constant for both the input and the assertion:
const logMessage = "message from derived logger"
derivedLogger.Info(logMessage)
if !strings.Contains(logOutput, logMessage) {
    t.Errorf("log output = %q, want it to contain %q", logOutput, logMessage)
}
6. Scenarios and Assertions
Choose scenarios based on the behavior and contract of the specific method.

As test coverage develops, verify:
- Valid input data.
- Invalid input data and expected errors.
- Boundary values.
- Empty values and states supported by the contract.
- Correct return values.
- Object state changes and the absence of unexpected side effects.
- Preservation of the original state after errors, when required by the contract.
Do not check only whether an error occurred. Check its type or identity when relevant to the contract. Use errors.Is() or errors.As() for wrapped errors when appropriate.

Assertions should make failures easy to diagnose. Include relevant actual and expected values in failure messages without making them unnecessarily verbose.

Do not duplicate production logic to calculate expected results. Specify expected values independently.

Do not invent additional business rules merely to increase coverage. Test the actual implementation contract, and report suspected defects separately.
7. Test Independence
Each test must be reproducible and, whenever possible, independent of the external environment.

For unit tests:
- Do not connect to PostgreSQL or other external services unless necessary.
- Do not use real HTTP requests or Docker to test pure business logic.
- Do not rely on test execution order.
- Avoid uncontrolled dependencies on the current time, random values, or mutable global state.
- Use t.Parallel() only when scenarios are guaranteed to be independent.
- Avoid shared mutable objects that allow one test to affect another.
Use fixed values for dates, times, and other inputs when possible. When testing behavior that inherently depends on the current time, make assertions that remain valid throughout the test execution.

External dependencies are acceptable in integration and end-to-end tests, but they must be explicitly defined and manageable.
8. File Placement and Naming
- Place tests alongside the source code in *_test.go files unless the project uses an established alternative for a particular test category.
- Follow the package's existing conventions when choosing test package names.
- Use the Test... naming convention for test functions.
- Give test functions and subtests names that identify the behavior being verified.
- Keep tests for unrelated methods in separate test functions.
9. Dependencies and Tools
Do not add third-party testing libraries without a justified need.

Start with Go's standard testing tools. If an additional library is genuinely necessary for a specific task, explain what problem it solves and why the standard library is insufficient.

Use mocks or fakes to isolate dependencies when appropriate. Do not use mocks as a substitute for testing real database queries, migrations, transactions, or other behavior that requires an actual dependency.

Choose the testing approach according to the behavior being verified:
- Unit tests: verify individual components in isolation.
- Integration tests: verify interactions between components and their dependencies.
- End-to-end tests: verify complete application workflows through an external interface.
Follow the project's established testing structure. Do not introduce additional infrastructure without a clear reason.
10. Verification
After writing or modifying tests:
1. Run gofmt on the modified Go files.
2. Run the tests for the target package.
3. Run go test ./... if the project state allows it.
4. Review the assertions and confirm that the tests verify the intended behavior.
5. Check that the tests are independent and that additional scenarios can be added easily.
6. Review the code for readability, clear scenario names, and consistent formatting.
7. Report which checks were executed, which passed, and which problems remain.
Do not claim that checks passed if the commands failed or were not executed.

If a test fails, investigate the cause before changing the test or production code. Do not weaken a valid assertion merely to make the test pass.
11. Gradually Improving Test Coverage
Do not attempt to implement every possible scenario in a single iteration.

First establish a simple and stable testing structure, then gradually expand it, focusing on risks and meaningful business logic.

For the internal/core/domain package, prioritize validation, calculations, patch application, state transitions, and constructors when they contain meaningful logic.

Prefer representative cases that cover significant behavior, error paths, and boundary conditions rather than enumerating every possible input.

Code coverage is a useful diagnostic metric, but it does not independently measure test quality. Testing quality is determined by how effectively tests detect defects and how easily they can be maintained as the project evolves.