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

A specialized AI agent that automates the release process for the ngrok Kubernetes Operator.

**What it does:**
- Creates release branches with proper naming conventions
- Updates version files (`VERSION` and `helm/ngrok-operator/Chart.yaml`)
- Runs Helm snapshot updates and tests
- Gathers PRs since the last release
- Updates both operator and Helm chart changelogs
- Creates commits and pull requests for releases

**How to use it:**

When working in a GitHub Copilot-enabled environment (VS Code, JetBrains, etc.), you can simply ask:

```
@release-agent create a release for version 0.20.0
```

Or more specifically:

```
@release-agent create a release with operator version 0.20.0 and helm chart version 0.22.0
```

The agent will:
1. Verify prerequisites (clean git tree, current versions)
2. Create a release branch
3. Update all necessary files
4. Run required tests
5. Update changelogs
6. Create a commit and push the changes
7. Guide you through creating a PR

**Manual Process:**

If you prefer to run the release manually, use:
```bash
make release
# or
./scripts/release.sh
```

For detailed documentation on the release process, see:
- `docs/developer-guide/releasing.md`
- `scripts/release.sh`

## About Custom Agents

Custom agents are part of GitHub Copilot's extensibility features. They provide repository-specific guidance and can automate complex, multi-step workflows.

For more information about GitHub Copilot custom agents, see:
- [GitHub Copilot Documentation](https://docs.github.com/en/copilot)
- [Adding custom instructions for GitHub Copilot](https://docs.github.com/en/copilot/customizing-copilot/adding-custom-instructions-for-github-copilot)

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
