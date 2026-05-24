package orchestrator

import (
	"fmt"
	"strings"
)

// BuildPipelinePrompt constructs the prompt for a pipeline stage task.
func BuildPipelinePrompt(stage, specContent, planContent, branchName, repoContext string) string {
	var b strings.Builder

	switch stage {
	case StageClarify:
		b.WriteString(clarifyPrompt(repoContext))
	case StagePlan:
		b.WriteString(planPrompt(specContent, repoContext))
	case StageImplement:
		b.WriteString(implementPrompt(specContent, planContent, branchName, repoContext))
	case StageValidate:
		b.WriteString(validatePrompt(branchName))
	case StageHandoff:
		b.WriteString(handoffPrompt(branchName))
	}

	return b.String()
}

func clarifyPrompt(repoContext string) string {
	return fmt.Sprintf(`You are a requirement clarification agent for the Conduit blog application.

## Your Task
Analyze the user's requirement and produce a structured specification. If the requirement is ambiguous, list the ambiguities and what assumptions you're making.

## Target Repository
%s

## Output Format
Write your analysis as a structured spec in the following format:

---
# Requirement Specification

## Summary
[1-2 sentence description of what needs to be done]

## Acceptance Criteria
- [ ] [criterion 1]
- [ ] [criterion 2]
...

## Technical Scope
- Frontend changes: [list affected components]
- Backend changes: [list affected routes/models]
- Database changes: [list migrations needed]

## Ambiguities Identified
- [ambiguity 1]: assumed [assumption]
...
---

Be thorough but concise. Focus on what's actionable.
`, repoContext)
}

func planPrompt(specContent, repoContext string) string {
	return fmt.Sprintf(`You are a technical planning agent for the Conduit blog application.

## Your Task
Based on the requirement specification below, create a technical implementation plan.

## Requirement Specification
%s

## Target Repository
%s

## Output Format
Produce a plan with:

1. **Implementation Steps** (ordered)
2. **Files to Modify** (with line-level precision where possible)
3. **New Files to Create** (if any)
4. **Risk Assessment** (what could go wrong)

Be specific about file paths and what changes are needed in each file.
Focus on the minimal set of changes to satisfy the requirements.
`, specContent, repoContext)
}

func implementPrompt(specContent, planContent, branchName, repoContext string) string {
	return fmt.Sprintf(`You are a code implementation agent for the Conduit blog application.

## Your Task
Implement the changes described in the plan below. You must modify actual files in the repository.

## Requirement
%s

## Implementation Plan
%s

## Working Branch
You are working on branch: %s

## Target Repository
%s

## Rules
1. Make the minimum changes necessary to satisfy the requirements
2. Follow existing code style and patterns in the repository
3. Ensure frontend and backend changes are consistent (API contracts match)
4. Do NOT break existing functionality
5. After making changes, commit with a meaningful message

## Steps
1. Read the files that need modification
2. Make the necessary code changes
3. Commit your changes with message: "feat: [description of change]"
`, specContent, planContent, branchName, repoContext)
}

func validatePrompt(branchName string) string {
	return fmt.Sprintf(`You are a validation agent for the Conduit blog application.

## Your Task
Run lint and tests on the current branch to validate the implementation.

## Working Branch
%s

## Steps
1. Run ESLint: npx eslint . --ext .js,.jsx 2>&1 | head -50
2. Run tests: npm test 2>&1 | head -100
3. Report results in this format:

---
## Validation Results

### Lint
- Status: PASS/FAIL
- Errors: [count]
- Warnings: [count]

### Tests
- Total: [count]
- Passed: [count]
- Failed: [count]
- Pass Rate: [percentage]

### Summary
[Brief assessment of code quality]
---

If there are lint errors or test failures, attempt to fix them (up to 2 attempts).
After fixing, re-run and report final results.
`, branchName)
}

func handoffPrompt(branchName string) string {
	return fmt.Sprintf(`You are a handoff agent for the Conduit blog application.

## Your Task
Create a Pull Request for the implementation on branch: %s

## Steps
1. Ensure all changes are committed
2. Push the branch to origin
3. Create a PR with:
   - Clear title describing the feature
   - Summary of changes
   - Test results
   - Any notes for reviewers

Report the PR URL when done.
`, branchName)
}
