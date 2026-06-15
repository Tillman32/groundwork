# Groundwork Documentation

[Home](index.md) | [Installation](installation.md) | [Configuration](configuration.md) | [Plugins](plugins.md) | [API](api.md)

## Quick Links

- [GitHub Repo](https://github.com/Tillman32/groundwork)
- [Releases](https://github.com/Tillman32/groundwork/releases)

---

## Overview

Groundwork is an agent/server system for autonomous infrastructure management.

### Components

| Component | Purpose |
|-----------|---------|
| `groundworkd` | Control plane server with REST API |
| `gw-agent` | Endpoint agent connected via WebSocket |
| `plugin` | Go/PowerShell plugin runtime |
| `registry` | Plugin discovery and caching |
| `notify` | Email notifications |
| `trust` | Trust scoring for agents/plugins |
| `policy` | Authorization policies |