# agentbox

Send a dirty local tree to a VPS and keep the run alive after the laptop sleeps. No git push.

The Mac talks to `agentboxd` over Tailscale SSH. Control goes through `devbox-control` (forced gateway, no shell). Source goes through `devbox-transfer` (rsync into a staging token only). The daemon activates an exact source generation, then starts a disposable systemd unit as a fresh Unix user.

Right now the VPS run is a fake provider that writes a marker and `sleep`s. Claude, Codex, and OMP are not wired on the box yet.

## Use it

Config lives in `~/.config/agentbox/config.toml`. SSH hosts `devbox-control` and `devbox-transfer` hold the keys and the Tailscale MSS proxy. You should not need env vars.

```sh
cd /path/to/project
agentbox init my-project
agentbox enroll
agentbox prepare
agentbox run --receipt r1 --provider fake
agentbox status
```

`prepare` builds a positive source manifest, rsyncs it, and activates a generation. `run --provider fake` starts `devbox-run-<id>.service`. Closing the Mac does not stop that unit.

## Build

```sh
cd agentbox
go build -o ~/.local/bin/agentbox ./cmd/agentbox
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o agentboxd ./cmd/agentboxd
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o devbox-gateway ./cmd/devbox-gateway
go test ./...
```

`agentboxd` listens on `/run/agentboxd/agentboxd.sock` (mode 0660, `root:devbox-control`). It has no TCP listener. Public SSH on the VPS is dropped; port 22 is Tailscale only.

## Layout

```
Mac                         VPS
agentbox  --ssh-->  devbox-gateway  -->  agentboxd.sock
          --rsync-->  /srv/devbox/staging/<token>/
                         generations/<project>/<id>/
                         runs/<id>/workspace
```

## Not in this tree

Provider credentials, Paseo on the VPS, ext4 project quotas. Hostinger does not expose `/dev/kvm`, so this is systemd-per-run, not a microVM.

## Design notes

[SPEC.md](SPEC.md), [REQUIREMENTS.md](REQUIREMENTS.md), [IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md), [VPS_AUDIT.md](VPS_AUDIT.md).
