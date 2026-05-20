# GitHub Copilot Agents

This directory contains custom GitHub Copilot agents that provide specialized assistance for repository-specific tasks.

## Available Agents

### Test Agent (`test-agent.agent.md`)

A specialized AI agent with expert knowledge of testing the ngrok Kubernetes Operator. It writes new tests following best practices, finds and fixes flaky tests, and verifies new tests are stable and well-structured.

**What it does:**
- Writes new tests following Ginkgo v2 / Gomega best practices for this codebase
- Verifies new tests are stable by running them repeatedly (`--repeat=N`) and with `-race`
- Reproduces flaky test failures and identifies root causes: missing `Eventually`, race conditions, shared state, insufficient timeouts
- Prefers fixing test code over production code unless a real bug is demonstrated
- Confirms fixes are stable by re-running tests multiple times

**How to use it:**

```
@test-agent write tests for the new MyController in internal/controller/ngrok
```

```
@test-agent investigate why TestFooController is flaky
```

```
@test-agent verify that the new tests I just added are not flaky
```

The agent will:
1. Analyze the code and existing test patterns
2. Write or fix tests following codebase conventions
3. Run tests repeatedly to confirm stability
4. Fix any flakiness found before finishing

---

### Release Agent (`release-agent.agent.md`)

A specialized AI agent that prepares ngrok Kubernetes Operator releases locally using the shared `release` skill.

**What it does:**
- Prefers a disposable dry-run worktree so release prep does not touch your current branch
- Uses the canonical shared skill in `.agents/skills/release/`
- Updates version files and changelogs for the operator, Helm chart, and CRDs chart when needed
- Runs Helm snapshot updates, Helm tests, and manifest bundle regeneration
- Leaves commit, push, and PR creation to the user unless explicitly requested

**How to use it:**

When working in a GitHub Copilot-enabled environment, ask:

```
@release-agent prepare a dry-run release rehearsal for the next operator release
```

Or, if you already know the target versions:

```
@release-agent prepare release changes with operator version 0.21.0 and helm chart version 0.23.0
```

The agent will:
1. Verify prerequisites (clean git tree, current versions)
2. Prefer a detached dry-run worktree
3. Update the necessary files
4. Run required validation
5. Update changelogs
6. Summarize the local diff and any manual next steps

**Shared skill and Claude support**

- Canonical skill: `.agents/skills/release/`
- Claude compatibility path: `.claude/skills/release` (symlink)
- Claude project guidance: `CLAUDE.md`
- Skill validation: `gh skill publish .agents --dry-run`

**Manual Process:**

If you prefer the legacy manual helper, use:
```bash
make release
```

For detailed documentation on the release process, see:
- `docs/developer-guide/releasing.md`
- `.agents/skills/release/SKILL.md`

## About Custom Agents

Custom agents are part of GitHub Copilot's extensibility features. They provide repository-specific guidance and can automate complex, multi-step workflows.

For more information about GitHub Copilot custom agents, see:
- [GitHub Copilot Documentation](https://docs.github.com/en/copilot)
- [Custom Agents Configuration Reference](https://docs.github.com/en/copilot/reference/custom-agents-configuration)

## Contributing

To add a new custom agent:

1. Create a new `.md` file in this directory
2. Follow the agent instruction format (see existing agents for examples)
3. Document the agent's purpose and usage in this README
4. Submit a PR for review

Agent files should:
- Have a clear, descriptive name (e.g., `deployment-agent.md`, `testing-agent.md`)
- Include comprehensive instructions for the AI
- Document common workflows and edge cases
- Provide examples where helpful
- Follow the repository's coding standards and best practices
