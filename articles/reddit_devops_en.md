# Built a TUI for managing kubectl port-forwards - no more terminal tab hell

**TL;DR:** Got tired of 10+ terminal tabs for port-forwards. Built a tool to manage them all in one place with session persistence.

## Problem

Every time I work on a project with multiple services:
```bash
# Tab 1: kubectl port-forward svc/postgres 5432:5432
# Tab 2: kubectl port-forward svc/redis 6379:6379  
# Tab 3: kubectl port-forward svc/api 8080:80
# Tab 4: kubectl port-forward pod/worker-abc123 9090:9090
# ... you get the idea
```

Then one drops, good luck finding which tab it was.

## Solution: PortFwd

Single terminal, all port-forwards visible:

```
● database/svc/postgres     localhost:5432 → 5432
● cache/svc/redis           localhost:6379 → 6379
○ api/svc/backend           localhost:8080 → 80    (stopped)
✗ dev/pod/worker            localhost:9090 → 9090  (error: pod not found)
```

### Features that actually matter:

**Session Persistence**
- Quit the app → reopen → connections restore
- Active ones reconnect, stopped ones stay in list
- State saved to `~/.config/portfwd/state.yaml`

**Smart Service Handling**
- Automatically resolves `targetPort` from Service spec
- Finds backing pod via selector
- No more "why is it connecting to port 80 when my app listens on 8000?"

**Per-Connection Logs**
- Press `l` to see logs for specific connection
- No more grepping through mixed output

**Graceful Everything**
- Clean shutdown (no zombie connections)
- Proper error handling with reconnect attempts

### Keybindings

```
n - new connection
d - disconnect  
r - reconnect
x - delete from list
l - view logs
? - help
q - quit
```

### Install

```bash
go install github.com/pyqan/portFwd@latest
# or
git clone ... && go build
```

### Tech

- Go + official client-go
- Bubble Tea for TUI
- Same SPDY transport as kubectl uses

### Not included (PRs welcome)

- [ ] Profiles/workspaces
- [ ] Import from kubectl commands
- [ ] Multi-cluster support

---

Would appreciate feedback. What would make this more useful for your daily workflow?

Repo: [github link]
