# NEX Control API
The NATS execution engine control API is a protocol by which clients can interact with and control NEX node processes. This protocol defines operations for things like **ping**, **info**, and **run**ning workloads.

Most of the protocol is fairly straightforward, but submitting a workload for execution requires a few pieces of information.
* **jwt** - Any workload publisher _must_ sign a set of claims containing the `hash` field, which asserts the issuer of a file with that specific hash.
* **Encrypted environment** - When sending a workload for execution, you'll typically need to set a number of environment variables (e.g. to establish a NATS or DB or HTTP connection). These environment variables contain sensitive information and so are not transmitted in plain text via NATS. They are encrypted with the **sender**'s Xkey, targeting the **recipient**'s Xkey. The recipient is the node to which the workload is being sent, and its public key can be obtained by querying the node's **info**.
* **Sender public Xkey** - the publisher needs to send its own public Xkey along in the request for execution so that the target node can decrypt the environment.

Manually taking these steps, either through the `nats` CLI or through your own code, can be tedious and error prone, so we recommend using this package for communicating with Nex nodes.