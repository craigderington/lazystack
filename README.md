# lazystack

A unified Terminal User Interface (TUI) for managing both systemd services and Kubernetes resources. Built with Go and [Bubbletea](https://github.com/charmbracelet/bubbletea).

## Features

- **Dual-pane interface**: Systemd services (left) and Kubernetes resources (right)
- **Real-time monitoring**: Auto-refreshing service and pod status (every 5 seconds)
- **Interactive lists**: Navigate and filter services/pods with Bubbles list component
- **Vim-style keybindings**: Familiar navigation for power users
- **Service management**: Start, stop, and restart systemd services with a single keystroke
- **Pod management**: Delete pods directly from the TUI
- **Status indicators**: Visual indicators for service states (active ●, inactive ○, failed ✗)
- **Filter/Search**: Built-in filtering to quickly find services or pods
- **Status messages**: Real-time feedback on actions and errors
- **Configuration**: Customizable via YAML config file (coming soon)

## Installation

### From Source

```bash
git clone https://github.com/craigderington/lazystack.git
cd lazystack
go build -o lazystack ./cmd/lazystack
sudo mv lazystack /usr/local/bin/
```

### Using Go Install

```bash
go install github.com/craigderington/lazystack@latest
```

## Usage

Simply run:

```bash
lazystack
```

### Keybindings

#### Global
- `tab` - Switch between systemd and k8s panes
- `h` - Focus systemd pane
- `l` - Focus k8s pane
- `j/k` - Navigate up/down in lists
- `r` - Refresh data manually
- `q` or `Ctrl+C` - Quit
- `/` - Filter/search items in the active list

#### Systemd Actions
- `s` - Start selected service
- `S` - Stop selected service (Shift+s)
- `R` - Restart selected service (Shift+r)

#### Kubernetes Actions
- `d` or `x` - Delete selected pod

## Configuration

Create a config file at `~/.config/lazystack/config.yaml`:

```yaml
systemd:
  units_to_watch:
    - postgresql.service
    - nginx.service
  auto_refresh_interval: 5s

kubernetes:
  kubeconfig: ~/.kube/config
  default_namespace: default
  auto_refresh_interval: 3s

ui:
  theme: default
  vim_mode: true
  split_ratio: 0.5
```

See [configs/config.example.yaml](configs/config.example.yaml) for a full example.

## Requirements

- **Linux**: systemd-based system (required for systemd features)
- **Kubernetes**: kubectl and kubeconfig (required for k8s features)
- **Go**: 1.21+ (for building from source)

## Architecture

lazystack uses the Bubbletea Elm architecture with three main components:

- **UI Layer**: Bubbletea models and views
- **Systemd Manager**: D-Bus integration for systemd control
- **K8s Manager**: client-go integration for Kubernetes control

## Development

### Using Make

The project includes a Makefile for common development tasks:

```bash
make build         # Build the application
make run           # Run the application
make test          # Run tests
make test-coverage # Run tests with coverage
make clean         # Clean build artifacts
make install       # Install to /usr/local/bin (requires sudo)
make lint          # Run linter
make fmt           # Format code
make vet           # Vet code
make tidy          # Tidy modules
make build-all     # Build for multiple platforms
make help          # Show all available targets
```

### Manual Build

```bash
go build -o lazystack ./cmd/lazystack
```

### Run Tests

```bash
go test ./...
# Or with coverage
go test -cover ./...
```

### Run Linter

```bash
golangci-lint run
```

## Inspiration

- [lazygit](https://github.com/jesseduffield/lazygit) - Git TUI
- [lazydocker](https://github.com/jesseduffield/lazydocker) - Docker TUI
- [k9s](https://github.com/derailed/k9s) - Kubernetes TUI

## License

MIT (pending)

## Contributing

Contributions welcome! Please open an issue or PR.
