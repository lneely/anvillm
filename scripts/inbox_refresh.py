#!/usr/bin/env python3
"""Refresh /AnviLLM/inbox acme window on UserRecv events."""

import json
import os
import subprocess
import time
from datetime import datetime

NAMESPACE = os.environ.get("NAMESPACE", f"/tmp/ns.{os.environ['USER']}.:0")

def find_inbox_window():
    """Find acme window ID for /AnviLLM/inbox."""
    try:
        ids = subprocess.run(["9p", "ls", "acme"], capture_output=True, text=True).stdout.strip().split()
        for wid in ids:
            tag = subprocess.run(["9p", "read", f"acme/{wid}/tag"], capture_output=True, text=True).stdout
            if tag.startswith("/AnviLLM/inbox"):
                return wid
    except Exception:
        pass
    return None

def read_user_inbox():
    """Read and format all messages in user inbox."""
    try:
        files = subprocess.run(["9p", "ls", "agent/user/inbox"], capture_output=True, text=True,
                               env={**os.environ, "NAMESPACE": NAMESPACE}).stdout.strip().split()
        files = [f for f in files if f.endswith(".json")]
    except Exception:
        return "Error reading inbox\n"
    
    lines = [
        "user/inbox Inbox",
        "=" * 120,
        "",
        f"{'ID':<10} {'Date':<20} {'From':<12} {'!':<1} {'Type':<18} {'Subject':<40}",
        "-" * 10 + " " + "-" * 20 + " " + "-" * 12 + " " + "-" + " " + "-" * 18 + " " + "-" * 40,
    ]
    
    msgs = []
    for f in files:
        try:
            data = subprocess.run(["9p", "read", f"agent/user/inbox/{f}"], capture_output=True, text=True,
                                  env={**os.environ, "NAMESPACE": NAMESPACE}).stdout
            msg = json.loads(data)
            msgs.append(msg)
        except Exception:
            pass
    
    msgs.sort(key=lambda m: m.get("timestamp", 0), reverse=True)
    
    for msg in msgs:
        mid = msg.get("id", "?")[:8]
        ts = msg.get("timestamp", 0)
        date = datetime.fromtimestamp(ts).strftime("%d-%b-%Y %H:%M:%S") if ts else "?"
        frm = msg.get("from", "?")[:12]
        mtype = msg.get("type", "?")[:18]
        subj = msg.get("subject", "")[:40]
        lines.append(f"{mid:<10} {date:<20} {frm:<12} {'':1} {mtype:<18} {subj:<40}")
    
    return "\n".join(lines) + "\n"

def clear_window(wid):
    """Clear window body."""
    subprocess.run(["9p", "write", f"acme/{wid}/addr"], input=b"0,$", capture_output=True)
    subprocess.run(["9p", "write", f"acme/{wid}/ctl"], input=b"dot=addr", capture_output=True)
    subprocess.run(f'9p write acme/{wid}/wrsel </dev/null', shell=True, capture_output=True)

def update_window(wid, content):
    """Clear window body and write new content."""
    clear_window(wid)
    subprocess.run(["9p", "write", f"acme/{wid}/addr"], input=b"0", capture_output=True)
    subprocess.run(["9p", "write", f"acme/{wid}/ctl"], input=b"dot=addr", capture_output=True)
    subprocess.run(["9p", "write", f"acme/{wid}/wrsel"], input=content.encode(), capture_output=True)
    subprocess.run(["9p", "write", f"acme/{wid}/addr"], input=b"0", capture_output=True)
    subprocess.run(["9p", "write", f"acme/{wid}/ctl"], input=b"show", capture_output=True)

if __name__ == "__main__":
    os.environ["NAMESPACE"] = NAMESPACE
    last_refresh = 0
    proc = subprocess.Popen(["9p", "read", "agent/events"], stdout=subprocess.PIPE, text=True,
                            env={**os.environ, "NAMESPACE": NAMESPACE})
    for line in proc.stdout:
        try:
            ev = json.loads(line.strip())
            if ev.get("type") == "UserRecv":
                now = time.time()
                if now - last_refresh < 1:
                    continue
                last_refresh = now
                wid = find_inbox_window()
                if wid:
                    content = read_user_inbox()
                    update_window(wid, content)
        except json.JSONDecodeError:
            pass
