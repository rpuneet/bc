# Demo Project: Fix the Greeting Bug

A minimal Go project for demonstrating bc multi-agent orchestration.

## The Bug

The `greet()` function returns "Hello" but should return "Welcome".

```bash
# Run the test to see the failure
go test -v
```

## Demo with bc

```bash
# Initialize bc workspace
bc up            # bootstraps the workspace

# Start the orchestration
bc up

# Create a GitHub issue for the fix task
gh issue create -t "Fix greeting bug" -b "greet() should return 'Welcome' instead of 'Hello'. Run tests to verify."

# Check agent status
bc status
```

## Expected Outcome

After bc processes the task:
1. An engineer agent picks up the work
2. Identifies the bug in greeting.go
3. Changes "Hello" to "Welcome"
4. Runs tests to verify the fix
5. Commits the change

## Verify the Fix

```bash
# Run tests - should pass after fix
go test -v

# Run the program
go run .
# Output: Welcome
```
