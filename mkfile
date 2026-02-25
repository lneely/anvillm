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
	cp scripts/anvillm-skills $INSTALL_PATH/anvillm-skills
	chmod 0755 $INSTALL_PATH/anvillm-skills
	cp scripts/anvillm-hook $INSTALL_PATH/anvillm-hook
	chmod 0755 $INSTALL_PATH/anvillm-hook
	cp scripts/9p-read-inbox $INSTALL_PATH/9p-read-inbox
	chmod 0755 $INSTALL_PATH/9p-read-inbox
	mkdir -p $INSTALL_PATH/Workflows
	cp bot-templates/* $INSTALL_PATH/Workflows
	chmod 0755 $INSTALL_PATH/Workflows/*
	mkdir -p $HOME/.config/anvillm
	cp -rf cfg/* $HOME/.config/anvillm/
	mkdir -p $HOME/.config/anvillm/mcptools
	cp cmd/anvilmcp/*.sh $HOME/.config/anvillm/mcptools/
	chmod 0755 $HOME/.config/anvillm/mcptools/*.sh
	mkdir -p $HOME/.kiro/agents/kiro-cli
	mkdir -p $HOME/.config/anvillm/claude/agents
	mkdir -p $HOME/.config/anvillm/claude/hooks
	cp kiro-cli/agent/* $HOME/.kiro/agents/
	cp kiro-cli/SKILLS_PROMPT.md $HOME/.kiro/
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

clean:V:
	rm -f $INSTALL_PATH/anvilsrv
	rm -f $INSTALL_PATH/anvilwebgw
	rm -f $INSTALL_PATH/Assist
