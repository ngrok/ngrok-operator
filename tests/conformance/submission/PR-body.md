**Title:** conformance: add ngrok Kubernetes Operator VERSION report and implementation entry

**What type of PR is this?**
/kind documentation

**What this PR does / why we need it**:

Adds a Gateway API conformance report for the [ngrok Kubernetes Operator](https://github.com/ngrok/ngrok-operator) and an `ImplementationListDetails` entry so it is listed on the Implementations page.

ngrok was previously listed (#2852, #3775, #3874) and was removed in the v1.5 Implementations Page review (#4741) because we had not submitted a conformance report. This PR fixes that.

The report is a standard-channel run against gateway-api v1.7.0 for operator VERSION, covering the GATEWAY-HTTP profile in default mode. Result: partially conformant: one core test is skipped, and the run needs a hostname suffix because ngrok hostnames are global. Both are explained in the report README.

Files:

- `conformance/list/implementations/ngrok-ngrok-operator/details.yaml`
- `conformance/reports/v1.7/ngrok-ngrok-operator/README.md`
- `conformance/reports/v1.7/ngrok-ngrok-operator/standard-VERSION-default-report.yaml`

**Which issue(s) this PR fixes**:

N/A

**Does this PR introduce a user-facing change?**:

```release-note
NONE
```

## AI disclosure

This PR was prepared with AI assistance. A maintainer of the ngrok Kubernetes Operator reviewed each line of the submission before opening it.
