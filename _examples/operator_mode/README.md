# Nex Operator Mode Example

> **⚠️ WARNING: DO NOT USE THIS IN PRODUCTION - SECRETS INCLUDED WITH REPO**

This example demonstrates running Nex with NATS in operator mode, including a 3-node NATS cluster with proper authentication and authorization.

## Overview

This setup creates a NATS cluster with:

- 3 NATS server nodes embedded in Nex
- Operator-based security model
- System account for Nex operations
- Individual node credentials

## Quick Start

### 1. Start the Nex Nodes

Start three Nex nodes, each with its own NATS server instance that will form a cluster:

#### Node 1

```bash
nex node up \
  --inats-config ./configs/node1.config \
  -s nats://0.0.0.0:10001 \
  --nats.creds-file creds/node1.creds \
  --issuer-signing-key-root-account AD3CQPQHV6EITMP4KBSZFY43T3GMGWPDJ2FD37DJZOLH5U3F7IIGW4J7 \
  --issuer-signing-key SAANMJG32UMZ7G3QN6XNJ3I2MQLXTYORY7NVD2OFTFDDWAUT2O6F2ONLGE \
  --logger.level debug
```

#### Node 2

```bash
nex node up \
  --inats-config ./configs/node2.config \
  -s nats://0.0.0.0:10002 \
  --nats.creds-file creds/node2.creds \
  --issuer-signing-key-root-account AD3CQPQHV6EITMP4KBSZFY43T3GMGWPDJ2FD37DJZOLH5U3F7IIGW4J7 \
  --issuer-signing-key SAANMJG32UMZ7G3QN6XNJ3I2MQLXTYORY7NVD2OFTFDDWAUT2O6F2ONLGE \
  --logger.level debug
```

#### Node 3

```bash
nex node up \
  --inats-config ./configs/node3.config \
  -s nats://0.0.0.0:10003 \
  --nats.creds-file creds/node3.creds \
  --issuer-signing-key-root-account AD3CQPQHV6EITMP4KBSZFY43T3GMGWPDJ2FD37DJZOLH5U3F7IIGW4J7 \
  --issuer-signing-key SAANMJG32UMZ7G3QN6XNJ3I2MQLXTYORY7NVD2OFTFDDWAUT2O6F2ONLGE \
  --logger.level debug
```

### 2. Verify the Cluster

Check NATS server status:

```bash
nats -s nats://0.0.0.0:10002 --creds creds/sys.creds server list
```

Expected output:

```
╭────────────────────────────────────────────────────────────────────────────────────────────────────────────────────╮
│                                                   Server Overview                                                  │
├──────┬─────────┬──────┬─────────┬─────┬───────┬──────┬────────┬─────┬────────┬───────┬───────┬──────┬────────┬─────┤
│ Name │ Cluster │ Host │ Version │ JS  │ Conns │ Subs │ Routes │ GWs │ Mem    │ CPU % │ Cores │ Slow │ Uptime │ RTT │
├──────┼─────────┼──────┼─────────┼─────┼───────┼──────┼────────┼─────┼────────┼───────┼───────┼──────┼────────┼─────┤
│ s2   │ cluster │ 0    │ 2.11.4  │ yes │ 3     │ 295  │      8 │   0 │ 28 MiB │ 0     │    20 │ 0    │ 5.41s  │ 1ms │
│ s3   │ cluster │ 0    │ 2.11.4  │ yes │ 2     │ 295  │      8 │   0 │ 29 MiB │ 0     │    20 │ 0    │ 3.68s  │ 1ms │
│ s1   │ cluster │ 0    │ 2.11.4  │ yes │ 2     │ 295  │      8 │   0 │ 29 MiB │ 0     │    20 │ 0    │ 6.46s  │ 1ms │
├──────┼─────────┼──────┼─────────┼─────┼───────┼──────┼────────┼─────┼────────┼───────┼───────┼──────┼────────┼─────┤
│      │ 1       │ 3    │         │ 3   │ 7     │ 885  │        │     │ 86 MiB │       │       │ 0    │        │     │
╰──────┴─────────┴──────┴─────────┴─────┴───────┴──────┴────────┴─────┴────────┴───────┴───────┴──────┴────────┴─────╯

╭────────────────────────────────────────────────────────────────────────────╮
│                              Cluster Overview                              │
├─────────┬────────────┬───────────────────┬───────────────────┬─────────────┤
│ Cluster │ Node Count │ Outgoing Gateways │ Incoming Gateways │ Connections │
├─────────┼────────────┼───────────────────┼───────────────────┼─────────────┤
│ cluster │          3 │                 0 │                 0 │           7 │
├─────────┼────────────┼───────────────────┼───────────────────┼─────────────┤
│         │          3 │                 0 │                 0 │           7 │
╰─────────┴────────────┴───────────────────┴───────────────────┴─────────────╯
```

Check Nex nodes:

```bash
nex -s nats://0.0.0.0:10002 --nats.creds-file creds/node1.creds node ls
```

Expected output:

```
╭─────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────╮
│                                                          Nex Nodes                                                          │
├───────┬──────────────────────────────────────────────────────────┬──────┬─────────┬──────────────┬─────────┬────────────────┤
│ Nexus │ ID (* = Lameduck Mode)                                   │ Name │ Version │ Uptime       │ State   │ Running Agents │
├───────┼──────────────────────────────────────────────────────────┼──────┼─────────┼──────────────┼─────────┼────────────────┤
│ nexus │ NAEUTL2GY5BF6KZVWSZNVPKPLNGHBBXAI5TE2MFX3W2CRXSHZHY7CRMT │      │ 0.0.0   │ 7.752221078s │ running │              1 │
│ nexus │ ND2T32CWEUJ5GJ3LBNBH4N2ALRBSLWRT7XD3FJEAHFVHJPGOLU6QZII7 │      │ 0.0.0   │ 8.796467594s │ running │              1 │
│ nexus │ NAWRKVDVWHWFA6BE2F25NOQFB4BYTDTHUVEH6LET5EM6JUB56PYQHB7D │      │ 0.0.0   │ 6.022330803s │ running │              1 │
╰───────┴──────────────────────────────────────────────────────────┴──────┴─────────┴──────────────┴─────────┴────────────────╯
```

## Environment Details

### Account Structure

- **`elastic_ellis`** - The operator (randomly generated name)
- **`SYS`** - System account for NATS operations
- **`system`** - Nex system account where nodes and nexlets operate
- **`user`** - Optional user account for non-system operations

### Key Hierarchy

View all keys with:

```bash
nsc -H ./nsc list keys -A
```

Expected output:

```
+---------------------------------------------------------------------------------------------------+
|                                               Keys                                                |
+-----------------+----------------------------------------------------------+-------------+--------+
| Entity          | Key                                                      | Signing Key | Stored |
+-----------------+----------------------------------------------------------+-------------+--------+
| elastic_ellis   | OCPLZ3M772PLGTTCDVEMW6SLDUAOHZEBRIH5MLNFUMLM7K4ONIELUR2Q |             | *      |
|  SYS            | ACWPJTAHYBAO7E3LKYN23HXK5HXDWBOA4VBGIOBRUALCOPWZUBWO4FUT |             | *      |
|  SYS            | ADV7UV5CLZGGPNGFATV4E3HDJGTVVNYBSHLZPCKGSXXX5DICRZH4W6ZJ | *           | *      |
|   sys           | UAODY6LIUISJNYCBJ6JZMVPEYHO3UKKEYZFFNC27AXFIIDIL7YM622TK |             | *      |
|  elastic_ellis  | AAC2L7M6SXWQ7RKHNUMNPWWTF7JTZC6HCQEWFFMQBYL7ZOUVRGHQXOBO |             | *      |
|   elastic_ellis | UD4X2Q6CYEDIXNIHH35RKWHEE7BZJTUS2I5OYT7OIVYFFC4WPEQHX2EP |             | *      |
|  system         | AD3CQPQHV6EITMP4KBSZFY43T3GMGWPDJ2FD37DJZOLH5U3F7IIGW4J7 |             | *      |
|  system         | ABCTZ6GID2WZSXQWEFSK4WRYILGHFHOARY6TNLE67V3DVRV23QOAVZOM | *           | *      |
|   node1         | UCLSOMMZI2NZJBPZYBBNRKZLSVKNDCMCQKK2FTLU4WERDPVU4YGCZULE |             | *      |
|   node2         | UA5BIQM6YYKU6PZHR2GCK46P2LVDTFILJTWKGXHJWK3DF5JNOXXSOUHK |             | *      |
|   node3         | UAABQ26ONJ5A2S5BZQRZV2PDO3C2J4K2T2POKUWGF7KCLM2BVPQIDATJ |             | *      |
|  user           | AD4BN74ECJ7YQYVPQZBTQBHJVAQHBTDRX4VXFKPZTTGAWBMBSA3DX7FC |             | *      |
+-----------------+----------------------------------------------------------+-------------+--------+
```

Key types:

- Operator keys (prefix: `O`)
- Account keys (prefix: `A`)
- User keys (prefix: `U`)
- Signing keys (marked with `*`)

## Recreating This Environment

### Step 1: Initialize Operator

```bash
nsc -H ./nsc init
```

### Step 2: Create System Account

```bash
nsc -H ./nsc add account system
```

### Step 3: Add Signing Key

```bash
nsc -H ./nsc edit account -n system --sk generate
```

### Step 4: Create Node Users

```bash
nsc -H ./nsc add user -a system node1
nsc -H ./nsc add user -a system node2
nsc -H ./nsc add user -a system node3
```

### Step 5: Generate Credentials

```bash
nsc -H ./nsc generate creds -a system -n node1 -o creds/node1.creds
nsc -H ./nsc generate creds -a system -n node2 -o creds/node2.creds
nsc -H ./nsc generate creds -a system -n node3 -o creds/node3.creds
```

### Step 6: Generate Resolver Config

```bash
nsc -H ./nsc generate config --mem-resolver --config-file resolver.config
```

### Step 7: Create Node Configs

Create individual NATS server configurations for each node following the templates in the `configs/` directory. Ensure you update:

- Signing key from the system account
- Root account public key
- Cluster routes
- Port configurations

## Important Notes

- The signing key is the one generated for the `system` account
- Each node needs unique ports to avoid conflicts
- All nodes must share the same signing key and root account
- The resolver configuration enables JWT-based authentication

