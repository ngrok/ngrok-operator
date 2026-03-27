# GitHub Copilot Agents

This directory contains custom GitHub Copilot agents that provide specialized assistance for repository-specific tasks.

## Available Agents

### Flakey Test Agent (`flakey-test-agent.agent.md`)

A specialized AI agent for identifying and fixing flakey (intermittently failing) tests in the ngrok Kubernetes Operator.

**What it does:**
- Reproduces flakey test failures by running tests repeatedly
- Identifies common causes: missing `Eventually`, race conditions, shared state, insufficient timeouts
- Prefers fixing test code over production code unless a real bug is demonstrated
- Verifies fixes are stable by re-running tests multiple times

**How to use it:**

```
@flakey-test-agent investigate why TestFooController is flakey
```

Or:

```
@flakey-test-agent fix the flakey tests in internal/controller/ingress
```

The agent will:
1. Run the tests multiple times to reproduce the failure
2. Analyze the test and production code to find the root cause
3. Fix the test (or production code if a real bug is found)
4. Re-run with `--repeat=20` or more to confirm the fix is stable

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
