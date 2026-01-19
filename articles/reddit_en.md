# [Tool] PortFwd - A TUI manager for kubectl port-forward that doesn't suck

Hey r/kubernetes!

I got tired of juggling multiple terminal windows for port-forwards, so I built **PortFwd** — a terminal UI manager that makes port-forwarding actually pleasant.

## The Pain

We've all been there:
- 5+ terminal tabs with different port-forwards
- "Which tab was postgres again?"
- Connection dropped, now you have to find and restart it
- Colleague asks for the command, you spend 2 minutes typing it out

## The Solution

PortFwd gives you:

**🖥️ Single pane of glass** - All your port-forwards in one terminal with status indicators

**🎯 Interactive selection** - Navigate with arrow keys: Namespace → Pod/Service → done

**💾 Session persistence** - Close the terminal, reopen, your connections are back (active ones reconnect automatically!)

**🔍 Smart port resolution** - Select a Service with port 80, app listens on 8000? PortFwd figures it out via targetPort

**📝 Per-connection logs** - Press `l` to see logs for any connection. No more output soup.

## Quick Demo

```
┌─ Connections ──────────────────────────────────┐
│ ● prod/svc/postgres              5432 → 5432   │
│ ● prod/svc/redis                 6379 → 6379   │
│ ○ staging/pod/api-server         3000 → 3000   │
│ ✗ dev/svc/frontend (conn refused) 8080 → 80   │
└────────────────────────────────────────────────┘
  ↑/↓ navigate │ n new │ d stop │ r reconnect │ l logs │ ? help
```

## Install

```bash
git clone https://github.com/pyqan/portFwd
cd portfwd
go build -o portfwd .
./portfwd
```

## Key bindings

- `n` - New port-forward
- `d` - Disconnect
- `r` - Reconnect
- `x` - Delete from list  
- `l` - View logs
- `?` - Help
- `q` - Quit (saves session!)

## Tech Stack

- Go + client-go (official K8s client)
- Bubble Tea (TUI framework by Charm)
- Uses same SPDY mechanism as kubectl

## What's NOT included (yet)

- Profiles/presets
- Multiple kubeconfig support
- Connection groups

Would love feedback! What features would make this more useful for your workflow?

GitHub: [link]

---

Edit: Thanks for the feedback! Already working on some suggested improvements.

---

*Crossposted to r/devops*
