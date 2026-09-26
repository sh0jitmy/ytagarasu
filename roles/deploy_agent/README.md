# Ansible Role: `deploy_agent`

Initial bootstrap Ansible role for the **Ytagarasu** autonomous offline deployment agent (`ytagarasu-agent`).

## Overview

This role is designed to be executed **only once** during initial server provisioning via SSH. After this role successfully sets up the environment, SSH access is no longer required: `ytagarasu-agent` polls the local offline artifact server (`ytagarasu-server`) autonomously.

## What this role configures

1. Dedicated system user & group (`deploy-agent`) with no login shell (`/usr/sbin/nologin`).
2. Directory hierarchy with secure permissions:
   - Configuration: `/etc/ytagarasu` (`0750`, `deploy-agent:deploy-agent`)
   - State & backups: `/var/lib/ytagarasu-agent` (`0750`)
   - Logs: `/var/log/ytagarasu-agent` (`0750`)
3. Agent binary placement (`/usr/local/bin/ytagarasu-agent`, `0755`).
4. Ed25519 Trust Anchor public key (`/etc/ytagarasu/release-signer.pub`).
5. Least-privilege `sudoers` configuration (`/etc/sudoers.d/deploy-agent`) verified with `visudo -cf`:
   - Non-interactive package operations (`apt-get`, `dnf`)
   - Service lifecycle reload and restart (`systemctl reload/restart`)
6. Hardened `systemd` service unit (`ytagarasu-agent.service`).

## Role Variables

| Variable | Default | Description |
|---|---|---|
| `deploy_agent_server_url` | `"http://127.0.0.1:8080"` | Base URL of `ytagarasu-server` |
| `deploy_agent_service_id` | `"default"` | Target service name assigned to host |
| `deploy_agent_poll_interval` | `"10s"` | Autonomous polling interval |
| `deploy_agent_roles` | `["api", "web"]` | Host roles matching manifest selectors |
| `deploy_agent_binary_src` | `"bin/ytagarasu-agent"` | Source binary path on control machine |
| `deploy_agent_release_signer_key` | `""` | Optional Ed25519 public key file |
| `deploy_agent_ca_cert` | `""` | Optional private CA certificate file |

## Example Playbook

```yaml
---
- hosts: target_servers
  become: true
  roles:
    - role: deploy_agent
      vars:
        deploy_agent_server_url: "http://192.168.10.10:8080"
        deploy_agent_service_id: "payment-api"
        deploy_agent_roles:
          - "api"
          - "backend"
```
