INSTALL_PATH=$HOME/bin

all:V: install

build:V:
	go build -o $INSTALL_PATH/anvillm .
	cp scripts/anvillm-notify $INSTALL_PATH/anvillm-notify
	chmod 0755 $INSTALL_PATH/anvillm-notify
	cp scripts/anvillm-hook $INSTALL_PATH/anvillm-hook
	chmod 0755 $INSTALL_PATH/anvillm-hook
	cp scripts/anvilspawn $INSTALL_PATH/anvilspawn
	chmod 0755 $INSTALL_PATH/anvilspawn
	cp scripts/anvillm-supervisor $INSTALL_PATH/anvillm-supervisor
	chmod 0755 $INSTALL_PATH/anvillm-supervisor
	mkdir -p $INSTALL_PATH/Teams
	cp -f team-templates/* $INSTALL_PATH/Teams/
	chmod 0755 $INSTALL_PATH/Teams/*
	mkdir -p $HOME/.config/anvillm
	cp -rf cfg/* $HOME/.config/anvillm/
	mkdir -p $HOME/.config/anvillm/roles
	cp -rf roles/* $HOME/.config/anvillm/roles/
	mkdir -p $HOME/.config/anvillm/estimation
	mkdir -p $HOME/.local/share/anvillm
	mkdir -p $HOME/.kiro/agents/kiro-cli
	cp kiro-cli/agent/* $HOME/.kiro/agents/
	mkdir -p $HOME/.config/anvillm/claude/agents
	mkdir -p $HOME/.config/anvillm/claude/hooks
	cp claude/agent/* $HOME/.config/anvillm/claude/agents/
	cp claude/hooks/* $HOME/.config/anvillm/claude/hooks/
	chmod +x $HOME/.config/anvillm/claude/hooks/*.sh
	bash claude/install-hooks.sh

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
	rm -f $INSTALL_PATH/Assist
