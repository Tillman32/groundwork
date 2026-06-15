# Plugins

Groundwork supports Go and PowerShell plugins.

## Plugin Types

| Runtime | Extension | Description |
|---------|-----------|-------------|
| go | `.so` | Go shared library plugin |
| powershell | `.ps1` | PowerShell script plugin |

## Plugin Manifest

Each plugin requires a `manifest.json`:

```json
{
  "name": "example-plugin",
  "version": "1.0.0",
  "runtime": "go",
  "entry": "plugin.so",
  "capabilities": ["filesystem", "network"],
  "author": "Your Name",
  "description": "Plugin description"
}
```

## Installing Plugins

### Via Registry
```bash
# Sync downloads and installs plugins
curl -X POST http://localhost:8080/api/v1/registry/sync
```

### Manual Install
```bash
# Copy to plugins directory
cp my-plugin.so /path/to/groundwork/plugins/
cp manifest.json /path/to/groundwork/plugins/
```

## Creating Plugins

### Go Plugin
```go
package main

import "github.com/Tillman32/groundwork/internal/plugin"

func Init() plugin.Plugin {
    return &MyPlugin{}
}

type MyPlugin struct{}

func (p *MyPlugin) Name() string { return "my-plugin" }
func (p *MyPlugin) Execute(ctx context.Context, args map[string]interface{}) (map[string]interface{}, error) {
    return map[string]interface{}{"result": "hello"}, nil
}
```