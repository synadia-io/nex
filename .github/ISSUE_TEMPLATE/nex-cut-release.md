---
name: Cut Pre-Release
about: Create a new release checklist
title: 'Cut VERSION'
labels: 'kind/release'
assignees: ''

---

**Summary:**
Task covering patch release work.

Dev Complete: RELEASE_DATE

**List of required releases:**
_To release as soon as able for QA:_
- VERSION

_To release once have approval from QA:_
- VERSION (Never release on a Friday unless specified otherwise)

**Prep work:**
- [ ] Dev and QA team to be notified of the incoming releases - add event to team calendar
- [ ] Dev and QA team to be notified of the date we will mark the latest release as stable - add event to team calendar [ONLY APPLICABLE FOR LATEST MINOR RELEASE]
- [ ] QA: Review changes and understand testing efforts
- [ ] Release Captain: Prepare release notes (submit PR for changes taking care to carefully check links and docs, once merged, create the release in GitHub and mark as a draft and check the pre-release box, fill in title, set target release branch, leave tag version blank for now until we are ready to release)
- [ ] QA: Validate and close out all issues in the release milestone. Ideally before the RC is cut.
- [ ] QA: Audit every PR has an associated issue for this release.

**release work:**
- [ ] Release Captain: Tag and release any necessary RCs for QA to test
- [ ] Release Captain: Tag and release when have QA approval

**Post-Release work:**
- [ ] Release Captain: Once release is fully complete (CI is all green and all release artifacts exist), edit the release, uncheck "Pre-release", and save.
- [ ] Release Captain: Prepare PRs as needed to update documentation for new features.
- [ ] Close the milestone in GitHub.

**Status Reference Table:**

| Status | Meaning                                                               |
| :----: | :-------------------------------------------------------------------- |
|   ⚪    | **Pending:** This section has not been cleared yet.                  |
|   🟢    | **Pass:** This section has been successfully cleared.                |
|   🟠    | **Skipped:** This section did not pass, but release should continue. |
|   🔴    | **Fail:** This section failed and the release should be aborted.     |

---

**QA Checklist**

| Status | Item                                   | Description                                                                                                                   |
| :----: | :--------------------------------------| :-----------------------------------------------------------------------------------------------------------------------------|
|   ⚪    | Prerequisites & Context               | List reasons for release, confirm expected commits are in the release.                                                        |
|   ⚪    | Nex Unit Tests Pipeline               | Run the full server tests suite against the release tag. Run the Lint,Test,Build GH workflow.                                 |
|   ⚪    | Multi-Auth Testing Matrix             | Run a matrix of workloads against a Nexus with a NATS cluster supporting various authentication modes.                        |
|   ⚪    | Long Running Chaos Test Matrix        | Run a matrix of workloads against a Nexus with a NATS cluster being subjected to various failure modes.                       |
|   ⚪    | Podman Nexlet Tests                   | Verify that the included release of synadia-io/nexlet.podman has passed all unit/integration tests with the race detector on. |
|   ⚪    | NGS Dev Manual Test                   | Deploy to NGS Dev and manually conduct tests to ensure UI and Nexus deployment are working together properly.                 |
|   ⚪    | Resource Utilization                  | Measure performance and resource utilization for future comparisons.                                                          |
|   ⚪    | Wilde Scenarios                       | Test any special deployments key stakeholders are utilizing.                                                                  |
---

**Detailed Instructions**

Skipped Suites:
    - If any of the tests are skipped an issue clearly stating the issue should be linked to the pre-release issue and be solved out of band until validated and closed.  

Prerequisites & Context:
    - Every commit should have a linked issue recording the change and the reason for the change as well as the validation steps to test the change. 

Nex Unit Tests Pipeline:
    - Running the pipeline built up around the project to ensure it's success. Verify lint, test, and builds work as expected and code quality adheres to standards.

Multi-Auth Testing Matrix:
    - Instructions:
            1. Open this pipeline: [Nex: Multi-Auth Testing Matrix](https://buildkite.com/synadia/nex-multi-auth-testing-matrix)
            2. In the top-right corner, hit ‘New Build’
            3. Fill the fields as per the example image below, notably:
                3a. Commit/Branch: leave default – these are references to the tests repository, not the server being tested
                3b. Set the following environment variables:
                    - NEX_VERSION (release branch name or tag)
                    - WORKLOAD_DURATION (optionally make this “5m”)

Long Running Chaos Test Matrix:
    - Instructions:
            1. Open this pipeline: [Nex: Single Node Testing Matrix](https://buildkite.com/synadia/nex-single-node-testing-matrix)
            2. In the top-right corner, hit ‘New Build’
            3. Fill the fields as per the example image below, notably:
                3a. Commit/Branch: leave default – these are references to the tests repository, not the server being tested
                3b. Set the following environment variables:
                    - NEX_VERSION (release branch name or tag)
                    - WORKLOAD_DURATION (make this a minimum of “30m”)

Podman Nexlet Tests:
    - Instructions:
           - Ideally this separate repository has passed it's own test suites apart from this issue but a quick check should suffice. https://github.com/synadia-io/nexlet.podman
           - Check for the corresponding tag of synadia-io/nexlet.podman and verify that the Go Build and Test GH Action has passed. 

NGS Dev Manual Test:
    - Instructions:
        1. Deploy release candidate tag/branch to NGS Dev
        2. Log in and navigate to Synadia Cloud Dev - Nexus User Test Account
        3. Conduct the following manual tests and verify expected behavior
            3a. Deploy a non-terminating workload
                - image: alpine
                - cmd: [“sleep”, “infinity”]
            3b. Deploy a terminating workload and verify it doesn’t exist after the sleep duration
                - image: alpine
                - cmd: [“sleep”, “5s”]
            3c. Deploy a function
