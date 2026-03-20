INSTALL_PATH=$HOME/bin

all:V: install

build:V:
	go build -o $INSTALL_PATH/anvillm .
	cp scripts/anvillm-hook $INSTALL_PATH/anvillm-hook
	chmod 0755 $INSTALL_PATH/anvillm-hook
	cp scripts/anvilspawn $INSTALL_PATH/anvilspawn
	chmod 0755 $INSTALL_PATH/anvilspawn
	cp scripts/anvillm-supervisor $INSTALL_PATH/anvillm-supervisor
	chmod 0755 $INSTALL_PATH/anvillm-supervisor
	mkdir -p $HOME/.config/anvillm
	cp -rf cfg/* $HOME/.config/anvillm/
	mkdir -p $HOME/.config/anvillm/roles
	cp -rf roles/* $HOME/.config/anvillm/roles/
	mkdir -p $HOME/.config/anvillm/estimation
	mkdir -p $HOME/.local/share/anvillm
	mkdir -p $HOME/.kiro/agents
	cp agents/kiro-cli/*.json $HOME/.kiro/agents/
	cp agents/OUTPUT_PROTOCOL.md agents/SKILLS_PROMPT.md agents/kiro-cli/USER.md $HOME/.kiro/
	mkdir -p $HOME/.config/anvillm/claude/agents
	cat agents/claude/anvillm-agent.md agents/OUTPUT_PROTOCOL.md agents/SKILLS_PROMPT.md agents/claude/USER.md > $HOME/.config/anvillm/claude/agents/anvillm-agent.md
	mkdir -p $HOME/.config/anvillm/claude/hooks
	cp hooks/claude/*.sh $HOME/.config/anvillm/claude/hooks/
	chmod +x $HOME/.config/anvillm/claude/hooks/*.sh
	bash hooks/claude/install-hooks.sh
	mkdir -p $HOME/.config/anvillm/tools
	cp tools/*.sh $HOME/.config/anvillm/tools/
	chmod +x $HOME/.config/anvillm/tools/*.sh

install:V: build

cron-install:V:
	mkdir -p $HOME/.local/share/anvillm
	crontab -l 2>/dev/null | grep -v 'anvillm-supervisor' >/tmp/crontab.new || true
	echo "*/5 * * * * export PLAN9=$PLAN9; export NAMESPACE=/tmp/ns.$USER.$DISPLAY; export PATH=$PATH; $HOME/bin/anvillm-supervisor --orphans >>$HOME/.local/share/anvillm/worker-check.log 2>&1" >>/tmp/crontab.new
	echo "*/1 * * * * export PLAN9=$PLAN9; export NAMESPACE=/tmp/ns.$USER.$DISPLAY; export PATH=$PATH; $HOME/bin/anvillm-supervisor --nudge >>$HOME/.local/share/anvillm/worker-check.log 2>&1" >>/tmp/crontab.new
	crontab /tmp/crontab.new
	rm /tmp/crontab.new
	echo "cron installed: anvillm-supervisor (orphans: every 5m, nudge: every 1m)"

cron-remove:V:
	(crontab -l 2>/dev/null | grep -v 'anvillm-supervisor') | crontab -
	echo "cron removed: anvillm-supervisor"

clean:V:
	rm -f $INSTALL_PATH/anvillm
