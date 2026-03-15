INSTALL_PATH=$HOME/bin

all:V: install

build:V:
	go build -o $INSTALL_PATH/anvilsrv ./cmd/anvilsrv
	go build -o $INSTALL_PATH/Assist ./cmd/Assist
	go build -o $INSTALL_PATH/anvillm ./cmd/anvillm
	go build -o $INSTALL_PATH/anvilweb ./cmd/anvilweb
	go build -o $INSTALL_PATH/anvilwebgw ./cmd/anvilwebgw
	go build -o $INSTALL_PATH/anvilmcp ./cmd/anvilmcp
	cp scripts/Alog $INSTALL_PATH/Alog
	chmod 0755 $INSTALL_PATH/Alog
	cp scripts/anvillm-notify $INSTALL_PATH/anvillm-notify
	chmod 0755 $INSTALL_PATH/anvillm-notify
	cp scripts/anvillm-skills $INSTALL_PATH/anvillm-skills
	chmod 0755 $INSTALL_PATH/anvillm-skills
	cp scripts/anvillm-hook $INSTALL_PATH/anvillm-hook
	chmod 0755 $INSTALL_PATH/anvillm-hook
	cp scripts/9p-read-inbox $INSTALL_PATH/9p-read-inbox
	chmod 0755 $INSTALL_PATH/9p-read-inbox
	cp scripts/anvilspawn $INSTALL_PATH/anvilspawn
	chmod 0755 $INSTALL_PATH/anvilspawn
	cp scripts/anvillm-supervisor $INSTALL_PATH/anvillm-supervisor
	chmod 0755 $INSTALL_PATH/anvillm-supervisor
	mkdir -p $INSTALL_PATH/Teams
	cp -f team-templates/* $INSTALL_PATH/Teams/
	chmod 0755 $INSTALL_PATH/Teams/*
	mkdir -p $HOME/.config/anvillm
	cp -rf cfg/* $HOME/.config/anvillm/
	mkdir -p $HOME/.config/anvillm/skills
	cp -rf skills/* $HOME/.config/anvillm/skills/
	find $HOME/.config/anvillm/skills -type f -name "*.sh" -exec chmod 0755 {} \;
	mkdir -p $HOME/.config/anvillm/roles
	cp -rf roles/* $HOME/.config/anvillm/roles/
	mkdir -p $HOME/.config/anvillm/estimation
	mkdir -p $HOME/.local/share/anvillm/estimation
	mkdir -p $HOME/.config/anvillm/mcptools
	cp -rf mcptools/* $HOME/.config/anvillm/mcptools/
	chmod 0755 $HOME/.config/anvillm/mcptools/*
	mkdir -p $HOME/.kiro/agents/kiro-cli
	mkdir -p $HOME/.config/anvillm/claude/agents
	mkdir -p $HOME/.config/anvillm/claude/hooks
	cp kiro-cli/agent/* $HOME/.kiro/agents/
	cp SKILLS_PROMPT.md $HOME/.kiro/
	mkdir -p $HOME/.config/anvillm/claude/agents/
	cp claude/agent/* $HOME/.config/anvillm/claude/agents/
	cp claude/hooks/* $HOME/.config/anvillm/claude/hooks/
	chmod +x $HOME/.config/anvillm/claude/hooks/*.sh
	bash claude/install-hooks.sh
	bash -c 'CLAUDE_CONFIG_DIR=$HOME/.config/anvillm/claude claude/install-mcp.sh'
	bash kiro-cli/install-mcp.sh
	cp ./ollama/mcp.json $HOME/.config/anvillm/ollama-mcp.json
	cp OUTPUT_PROTOCOL.md $HOME/.kiro
	cp OUTPUT_PROTOCOL.md $HOME/.config/anvillm/claude

install:V: build

cron-install:V:
	mkdir -p $HOME/.local/share/anvillm
	crontab -l 2>/dev/null | grep -v 'anvillm-supervisor' | grep -v '^PLAN9=' | grep -v '^PATH=' >/tmp/crontab.new || true
	echo "PLAN9=$PLAN9" >>/tmp/crontab.new
	echo "PATH=$PATH" >>/tmp/crontab.new
	echo "*/5 * * * * $HOME/bin/anvillm-supervisor --orphans >>$HOME/.local/share/anvillm/worker-check.log 2>&1" >>/tmp/crontab.new
	echo "*/1 * * * * $HOME/bin/anvillm-supervisor --nudge >>$HOME/.local/share/anvillm/worker-check.log 2>&1" >>/tmp/crontab.new
	crontab /tmp/crontab.new
	rm /tmp/crontab.new
	echo "cron installed: anvillm-supervisor (orphans: every 5m, nudge: every 1m)"

cron-remove:V:
	(crontab -l 2>/dev/null | grep -v 'anvillm-supervisor') | crontab -
	echo "cron removed: anvillm-supervisor"

clean:V:
	rm -f $INSTALL_PATH/anvilsrv
	rm -f $INSTALL_PATH/anvilwebgw
	rm -f $INSTALL_PATH/Assist
