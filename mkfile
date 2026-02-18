INSTALL_PATH=$HOME/bin

all:V: install

build:V:
	go build -o $INSTALL_PATH/anvilsrv ./cmd/anvilsrv
	go build -o $INSTALL_PATH/Assist ./cmd/Assist
	go build -o $INSTALL_PATH/anvillm ./cmd/anvillm
	go build -o $INSTALL_PATH/anvilweb ./cmd/anvilweb
	go build -o $INSTALL_PATH/anvilmcp ./cmd/anvilmcp
	cp scripts/Alog $INSTALL_PATH/Alog
	chmod 0755 $INSTALL_PATH/Alog
	cp scripts/anvillm-skills $INSTALL_PATH/anvillm-skills
	chmod 0755 $INSTALL_PATH/anvillm-skills
	cp scripts/9p-read-inbox $INSTALL_PATH/9p-read-inbox
	chmod 0755 $INSTALL_PATH/9p-read-inbox
	mkdir -p $INSTALL_PATH/Workflows
	cp workflows/* $INSTALL_PATH/Workflows
	chmod 0755 $INSTALL_PATH/Workflows/*
	mkdir -p $HOME/.config/anvillm
	cp -rf cfg/* $HOME/.config/anvillm/
	mkdir -p $HOME/.kiro/agents/kiro-cli
	mkdir -p $HOME/.claude/agents
	mkdir -p $HOME/.claude/hooks
	cp kiro-cli/agent/* $HOME/.kiro/agents/kiro-cli/
	cp claude/agent/* $HOME/.claude/agents/
	cp claude/hooks/* $HOME/.claude/hooks/
	chmod +x $HOME/.claude/hooks/*.sh
	bash claude/install-hooks.sh

install:V: build

clean:V:
	rm -f $INSTALL_PATH/anvilsrv
	rm -f $INSTALL_PATH/Assist
