#!/usr/bin/env python3
"""AnviLLM message trace tool - PlantUML sequence diagram generator."""

import json
import os
import subprocess
import threading
from http.server import HTTPServer, BaseHTTPRequestHandler

NAMESPACE = os.environ.get("NAMESPACE", f"/tmp/ns.{os.environ['USER']}.:0")
PORT = 8089

events = []
sessions = {}  # id -> (alias, role)
lock = threading.Lock()

def format_participant(pid):
    """Format participant with alias and role if available."""
    if pid == "user":
        return "user"
    info = sessions.get(pid)
    if info:
        alias, role = info
        if alias and alias != "-":
            label = alias
        else:
            label = pid
        if role and role != "-":
            return f"{label}\\n({role})"
        return label
    return pid

def generate_puml():
    with lock:
        lines = ["@startuml", "skinparam responseMessageBelowArrow true", "participant user"]
        for e in events:
            d = e.get("data", {})
            frm = format_participant(d.get("from", "?"))
            to = format_participant(d.get("to", "?"))
            subj = d.get("subject", "").replace('"', "'")[:60]
            lines.append(f'"{frm}" -> "{to}": {subj}')
        lines.append("@enduml")
    return "\n".join(lines)

def render_svg(puml):
    try:
        p = subprocess.run(["plantuml", "-tsvg", "-pipe"], input=puml.encode(),
                           capture_output=True, timeout=10)
        return p.stdout.decode() if p.returncode == 0 else f"<pre>Error: {p.stderr.decode()}</pre>"
    except Exception as ex:
        return f"<pre>PlantUML error: {ex}</pre>"

HTML = """<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>AnviLLM Trace</title>
<style>
body{margin:0;display:flex;height:100vh;font-family:monospace}
#left{width:25%;overflow:auto;background:#1e1e1e;color:#d4d4d4;padding:8px;box-sizing:border-box;display:flex;flex-direction:column}
#right{width:75%;overflow:auto;background:#fff;display:flex;align-items:center;justify-content:center}
#svg-container{transform-origin:center;cursor:grab}
pre{margin:0;white-space:pre-wrap;font-size:12px;flex:1}
button{margin-bottom:8px;padding:4px 8px;cursor:pointer}
</style></head><body>
<div id="left"><button onclick="reset()">Reset</button><pre id="puml"></pre></div>
<div id="right"><div id="svg-container"></div></div>
<script>
let scale=1;
function refresh(){
  fetch('/data').then(r=>r.json()).then(d=>{
    document.getElementById('puml').textContent=d.puml;
    document.getElementById('svg-container').innerHTML=d.svg;
  });
}
function reset(){fetch('/reset',{method:'POST'}).then(()=>{scale=1;refresh();});}
document.getElementById('right').addEventListener('wheel',e=>{
  e.preventDefault();
  scale*=e.deltaY<0?1.1:0.9;
  document.getElementById('svg-container').style.transform='scale('+scale+')';
});
setInterval(refresh,2000);
refresh();
</script></body></html>"""

class Handler(BaseHTTPRequestHandler):
    def log_message(self, *a): pass
    def do_GET(self):
        if self.path == "/":
            self.send_response(200)
            self.send_header("Content-Type", "text/html")
            self.end_headers()
            self.wfile.write(HTML.encode())
        elif self.path == "/data":
            puml = generate_puml()
            svg = render_svg(puml)
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps({"puml": puml, "svg": svg}).encode())
        else:
            self.send_error(404)
    def do_POST(self):
        if self.path == "/reset":
            with lock:
                events.clear()
            self.send_response(200)
            self.end_headers()
        else:
            self.send_error(404)

def event_reader():
    os.environ["NAMESPACE"] = NAMESPACE
    proc = subprocess.Popen(["9p", "read", "anvillm/events"], stdout=subprocess.PIPE, text=True)
    for line in proc.stdout:
        try:
            ev = json.loads(line.strip())
            if ev.get("type") in ("UserSend", "BotSend"):
                with lock:
                    events.append(ev)
        except json.JSONDecodeError:
            pass

def refresh_sessions():
    global sessions
    while True:
        try:
            p = subprocess.run(["9p", "read", "anvillm/list"], capture_output=True, text=True, timeout=5, env={**os.environ, "NAMESPACE": NAMESPACE})
            if p.returncode == 0:
                new_sessions = {}
                for line in p.stdout.strip().split("\n"):
                    if not line:
                        continue
                    parts = line.split("\t")
                    if len(parts) >= 5:
                        sid, _, _, alias, role = parts[:5]
                        new_sessions[sid] = (alias, role)
                with lock:
                    sessions.update(new_sessions)
        except Exception:
            pass
        threading.Event().wait(2)

if __name__ == "__main__":
    threading.Thread(target=event_reader, daemon=True).start()
    threading.Thread(target=refresh_sessions, daemon=True).start()
    print(f"Trace UI: http://localhost:{PORT}")
    HTTPServer(("", PORT), Handler).serve_forever()
